package mysql

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/errs"
	"github.com/ceymard/swl-go/internal/handlers"
)

const insertBatchSize = 512

// Sink writes collections into MySQL using batched INSERT statements.
type Sink struct{}

func (Sink) Sink(ctx context.Context, cfg handlers.Config, in coll.Stream, raw any) error {
	opts := raw.(SinkOpts)
	if opts.URI == "" {
		return errs.New("mysql sink requires a connection URI")
	}
	h := &sinkHooks{cfg: cfg, opts: opts}
	return handlers.ConsumeHooks(cfg, h, in)
}

type sinkHooks struct {
	cfg  handlers.Config
	opts SinkOpts
	db   *sql.DB
	tun  interface{ Close() error }
	tx   *sql.Tx
}

func (h *sinkHooks) Init(ctx context.Context) error {
	db, tun, err := connect(ctx, h.opts.URI)
	if err != nil {
		return err
	}
	h.db = db
	h.tun = tun
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		_ = db.Close()
		_ = tun.Close()
		return errs.Wrap(err, "mysql begin")
	}
	h.tx = tx
	if h.cfg.Messages != nil {
		h.cfg.Log(1, "connected to", h.cfg.ConnTarget(h.opts.URI))
	}
	return nil
}

func (h *sinkHooks) Open(ctx context.Context, col coll.Collection, firstRow coll.Row) (handlers.RowWriter, error) {
	table := col.Name
	cols := columnNames(col)

	if h.opts.Drop {
		stmt := "DROP TABLE IF EXISTS " + quoteTable(table)
		if h.cfg.Messages != nil {
			h.cfg.Log(2, "dropping table", table)
			h.cfg.Log(3, stmt)
		}
		if _, err := h.tx.ExecContext(ctx, stmt); err != nil {
			return nil, errs.Wrap(err, "drop table", "table", table)
		}
	}

	if h.opts.AutoCreate || h.opts.Drop {
		ddl := buildCreateTable(table, cols, firstRow)
		if h.cfg.Messages != nil {
			h.cfg.Log(3, ddl)
		}
		if _, err := h.tx.ExecContext(ctx, ddl); err != nil {
			return nil, errs.Wrap(err, "create table", "table", table)
		}
	}

	if h.opts.Truncate {
		stmt := "TRUNCATE TABLE " + quoteTable(table)
		if h.cfg.Messages != nil {
			h.cfg.Log(2, "truncating table", table)
			h.cfg.Log(3, stmt)
		}
		if _, err := h.tx.ExecContext(ctx, stmt); err != nil {
			return nil, errs.Wrap(err, "truncate table", "table", table)
		}
	}

	w := &batchWriter{
		tx:     h.tx,
		cfg:    h.cfg,
		table:  table,
		cols:   cols,
		upsert: h.opts.Upsert,
	}
	if err := w.append(firstRow); err != nil {
		return nil, err
	}
	return w, nil
}

func (h *sinkHooks) Rollback(ctx context.Context) {
	if h.tx != nil {
		_ = h.tx.Rollback()
	}
	if h.db != nil {
		_ = h.db.Close()
	}
	if h.tun != nil {
		_ = h.tun.Close()
	}
	if h.cfg.Messages != nil {
		h.cfg.Log(2, "rollbacked mysql transaction")
	}
}

func (h *sinkHooks) Finish(ctx context.Context) error {
	if h.tx != nil {
		if err := h.tx.Commit(); err != nil {
			return errs.Wrap(err, "mysql commit")
		}
	}
	if h.db != nil {
		if err := h.db.Close(); err != nil {
			return errs.Wrap(err, "close mysql database")
		}
	}
	if h.tun != nil {
		_ = h.tun.Close()
	}
	if h.cfg.Messages != nil {
		h.cfg.Log(2, "committed mysql changes")
	}
	return nil
}

type batchWriter struct {
	tx      *sql.Tx
	cfg     handlers.Config
	table   string
	cols    []string
	upsert  bool
	pending []coll.Row
	args    []any
}

func (w *batchWriter) Write(row coll.Row) error {
	return w.append(row)
}

func (w *batchWriter) Close() error {
	if err := w.flush(); err != nil {
		return err
	}
	return nil
}

func (w *batchWriter) append(row coll.Row) error {
	w.pending = append(w.pending, row)
	if len(w.pending) >= insertBatchSize {
		return w.flush()
	}
	return nil
}

func (w *batchWriter) flush() error {
	if len(w.pending) == 0 {
		return nil
	}

	quotedCols := make([]string, len(w.cols))
	for i, c := range w.cols {
		quotedCols[i] = quoteColumn(c)
	}
	placeholders := make([]string, len(w.pending))
	needArgs := len(w.cols) * len(w.pending)
	if cap(w.args) < needArgs {
		w.args = make([]any, needArgs)
	}
	args := w.args[:needArgs]

	argIdx := 0
	for i, row := range w.pending {
		ph := make([]string, len(w.cols))
		for j, c := range w.cols {
			v, err := bindValue(row.Cell(j))
			if err != nil {
				return errs.Wrap(err, "bind value", "column", c)
			}
			args[argIdx] = v
			ph[j] = "?"
			argIdx++
		}
		placeholders[i] = "(" + strings.Join(ph, ", ") + ")"
	}

	stmt := fmt.Sprintf("INSERT INTO %s (%s) VALUES %s",
		quoteTable(w.table),
		strings.Join(quotedCols, ", "),
		strings.Join(placeholders, ", "),
	)
	if w.upsert {
		updates := make([]string, len(w.cols))
		for i, c := range w.cols {
			qc := quoteColumn(c)
			updates[i] = qc + "=VALUES(" + qc + ")"
		}
		stmt += " ON DUPLICATE KEY UPDATE " + strings.Join(updates, ", ")
	}
	if w.cfg.Messages != nil {
		w.cfg.Log(3, stmt)
	}
	if _, err := w.tx.ExecContext(context.Background(), stmt, args...); err != nil {
		return errs.Wrap(err, "insert batch", "table", w.table)
	}
	w.pending = w.pending[:0]
	return nil
}

func buildCreateTable(table string, cols []string, sample coll.Row) string {
	parts := make([]string, len(cols))
	for i, c := range cols {
		parts[i] = quoteColumn(c) + " " + inferColumnType(sample.Cell(i))
	}
	return fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (%s)",
		quoteTable(table), strings.Join(parts, ", "))
}
