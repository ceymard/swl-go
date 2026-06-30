package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/errs"
	"github.com/ceymard/swl-go/internal/handlers"

	_ "modernc.org/sqlite"
)

// Sink writes collections into a SQLite database using runner.ConsumeHooks.
type Sink struct{}

func (Sink) Sink(ctx context.Context, cfg handlers.Config, in coll.Stream, raw any) error {
	opts := raw.(SinkOpts)
	if opts.File == "" {
		return errs.New("sqlite sink requires a database file path")
	}
	h := &sinkHooks{cfg: cfg, opts: opts}
	return handlers.ConsumeHooks(cfg, h, in)
}

type sinkHooks struct {
	cfg  handlers.Config
	opts SinkOpts
	db   *sql.DB
	tx   *sql.Tx
}

func (h *sinkHooks) Init(ctx context.Context) error {
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s", h.opts.File))
	if err != nil {
		return errs.Wrap(err, "open sqlite database", "path", h.opts.File)
	}
	h.db = db
	if _, err := h.db.ExecContext(ctx, `PRAGMA journal_mode = wal`); err != nil {
		_ = db.Close()
		return errs.Wrap(err, "sqlite pragma journal_mode")
	}
	if _, err := h.db.ExecContext(ctx, `PRAGMA synchronous = 0`); err != nil {
		_ = db.Close()
		return errs.Wrap(err, "sqlite pragma synchronous")
	}
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		_ = db.Close()
		return errs.Wrap(err, "sqlite begin")
	}
	h.tx = tx
	if h.cfg.Messages != nil {
		h.cfg.Messages.Log(2, "opened sqlite database", h.opts.File, "to write")
	}
	return nil
}

func (h *sinkHooks) Open(ctx context.Context, col coll.Collection, firstRow coll.Row) (handlers.RowWriter, error) {
	table := col.Name
	cols := columnNames(firstRow)

	if h.opts.Drop {
		if h.cfg.Messages != nil {
			h.cfg.Messages.Log(2, "dropping table", table)
		}
		if _, err := h.tx.ExecContext(ctx, fmt.Sprintf(`DROP TABLE IF EXISTS "%s"`, table)); err != nil {
			return nil, errs.Wrap(err, "drop table", "table", table)
		}
	}

	ddl := buildCreateTable(table, cols, firstRow)
	if _, err := h.tx.ExecContext(ctx, ddl); err != nil {
		return nil, errs.Wrap(err, "create table", "table", table)
	}

	if h.opts.Truncate {
		if h.cfg.Messages != nil {
			h.cfg.Messages.Log(2, "truncating table", table)
		}
		if _, err := h.tx.ExecContext(ctx, fmt.Sprintf(`DELETE FROM "%s"`, table)); err != nil {
			return nil, errs.Wrap(err, "truncate table", "table", table)
		}
	}

	insertSQL := buildInsert(table, cols, h.opts.Upsert)
	stmt, err := h.tx.PrepareContext(ctx, insertSQL)
	if err != nil {
		return nil, errs.Wrap(err, "prepare insert", "table", table)
	}

	w := &rowWriter{
		tx:    h.tx,
		stmt:  stmt,
		cols:  cols,
		table: table,
		cfg:   h.cfg,
	}
	if err := w.insert(firstRow); err != nil {
		_ = stmt.Close()
		return nil, err
	}
	return w, nil
}

func (h *sinkHooks) Rollback(ctx context.Context) {
	if h.tx != nil {
		_ = h.tx.Rollback()
	}
	if h.db != nil {
		_, _ = h.db.ExecContext(ctx, `PRAGMA journal_mode = delete`)
		_ = h.db.Close()
	}
	if h.cfg.Messages != nil {
		h.cfg.Messages.Log(2, "rollbacked sqlite transaction")
	}
}

func (h *sinkHooks) Finish(ctx context.Context) error {
	if h.tx != nil {
		if err := h.tx.Commit(); err != nil {
			return errs.Wrap(err, "sqlite commit")
		}
	}
	if h.db != nil {
		_, _ = h.db.ExecContext(ctx, `PRAGMA journal_mode = delete`)
		if err := h.db.Close(); err != nil {
			return errs.Wrap(err, "close sqlite database")
		}
	}
	if h.cfg.Messages != nil {
		h.cfg.Messages.Log(2, "committed sqlite changes")
	}
	return nil
}

type rowWriter struct {
	tx    *sql.Tx
	stmt  *sql.Stmt
	cols  []string
	table string
	cfg   handlers.Config
}

func (w *rowWriter) Write(row coll.Row) error {
	return w.insert(row)
}

func (w *rowWriter) Close() error {
	if w.cfg.Messages != nil && w.cfg.Verbose >= 2 && w.tx != nil {
		var n int64
		q := fmt.Sprintf(`SELECT count(*) FROM "%s"`, w.table)
		if err := w.tx.QueryRowContext(context.Background(), q).Scan(&n); err == nil {
			w.cfg.Messages.Log(2, "table", w.table, "now has", n, "rows")
		}
	}
	if err := w.stmt.Close(); err != nil {
		return errs.Wrap(err, "close insert stmt", "table", w.table)
	}
	return nil
}

func (w *rowWriter) insert(row coll.Row) error {
	args := make([]any, len(w.cols))
	for i, c := range w.cols {
		v, err := bindValue(row[c])
		if err != nil {
			return errs.Wrap(err, "bind value", "column", c)
		}
		args[i] = v
	}
	if _, err := w.stmt.Exec(args...); err != nil {
		return errs.Wrap(err, "insert row", "table", w.table)
	}
	return nil
}

func buildCreateTable(table string, cols []string, sample coll.Row) string {
	parts := make([]string, len(cols))
	for i, c := range cols {
		parts[i] = fmt.Sprintf(`"%s" %s`, c, inferColumnType(sample[c]))
	}
	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS "%s" (%s)`, table, strings.Join(parts, ", "))
}

func buildInsert(table string, cols []string, upsert bool) string {
	quoted := make([]string, len(cols))
	placeholders := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = fmt.Sprintf(`"%s"`, c)
		placeholders[i] = "?"
	}
	kw := "INSERT INTO"
	if upsert {
		kw = "INSERT OR REPLACE INTO"
	}
	return fmt.Sprintf(`%s "%s" (%s) VALUES (%s)`,
		kw, table, strings.Join(quoted, ", "), strings.Join(placeholders, ", "))
}
