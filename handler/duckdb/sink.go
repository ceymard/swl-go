package duckdb

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/errs"
	"github.com/ceymard/swl-go/internal/handlers"
	"github.com/ceymard/swl-go/internal/jsonx"
)

const jsonBatchSize = 1024

// Sink writes collections into a DuckDB database (swl2 duckdb-sink-common.ts).
type Sink struct{}

func (Sink) Sink(ctx context.Context, cfg handlers.Config, in coll.Stream, raw any) error {
	opts := raw.(SinkOpts)
	if opts.File == "" {
		return errs.New("duckdb sink requires a database file path")
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
	db, err := sql.Open(driverName, h.opts.File)
	if err != nil {
		return errs.Wrap(err, "open duckdb database", "path", h.opts.File)
	}
	h.db = db
	tx, err := h.db.BeginTx(ctx, nil)
	if err != nil {
		_ = db.Close()
		return errs.Wrap(err, "duckdb begin")
	}
	h.tx = tx
	if h.cfg.Messages != nil {
		h.cfg.Log(2, "opened duckdb database", h.cfg.ConnTarget(h.opts.File), "to write")
	}
	return nil
}

func (h *sinkHooks) Open(ctx context.Context, col coll.Collection, firstRow coll.Row) (handlers.RowWriter, error) {
	schema, table := splitSchemaTable(col.Name)
	cols := columnNames(firstRow)

	if schema != "main" {
		stmt := fmt.Sprintf(`CREATE SCHEMA IF NOT EXISTS %s`, quoteIdent(schema))
		if h.cfg.Messages != nil {
			h.cfg.Log(3, stmt)
		}
		if _, err := h.tx.ExecContext(ctx, stmt); err != nil {
			return nil, errs.Wrap(err, "create schema", "schema", schema)
		}
	}

	if h.opts.Drop {
		stmt := fmt.Sprintf(`DROP TABLE IF EXISTS %s.%s`, quoteIdent(schema), quoteIdent(table))
		if h.cfg.Messages != nil {
			h.cfg.Log(2, "dropping table", col.Name)
			h.cfg.Log(3, stmt)
		}
		if _, err := h.tx.ExecContext(ctx, stmt); err != nil {
			return nil, errs.Wrap(err, "drop table", "table", col.Name)
		}
	}

	ddl := buildCreateTable(schema, table, cols, firstRow)
	if h.cfg.Messages != nil {
		h.cfg.Log(3, ddl)
	}
	if _, err := h.tx.ExecContext(ctx, ddl); err != nil {
		return nil, errs.Wrap(err, "create table", "table", col.Name)
	}

	if h.opts.Truncate {
		stmt := fmt.Sprintf(`DELETE FROM %s.%s`, quoteIdent(schema), quoteIdent(table))
		if h.cfg.Messages != nil {
			h.cfg.Log(2, "truncating table", col.Name)
			h.cfg.Log(3, stmt)
		}
		if _, err := h.tx.ExecContext(ctx, stmt); err != nil {
			return nil, errs.Wrap(err, "truncate table", "table", col.Name)
		}
	}

	structType, err := tableStructType(ctx, h.tx, schema, table)
	if err != nil {
		return nil, err
	}

	if _, err := h.tx.ExecContext(ctx, `CREATE TEMP TABLE __temp__json (data VARCHAR)`); err != nil {
		return nil, errs.Wrap(err, "create temp json table")
	}

	w := &collectionWriter{
		tx:         h.tx,
		cfg:        h.cfg,
		schema:     schema,
		table:      table,
		cols:       cols,
		structType: structType,
		upsert:     h.opts.Upsert,
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
	if h.cfg.Messages != nil {
		h.cfg.Log(2, "rollbacked duckdb transaction")
	}
}

func (h *sinkHooks) Finish(ctx context.Context) error {
	if h.tx != nil {
		if err := h.tx.Commit(); err != nil {
			return errs.Wrap(err, "duckdb commit")
		}
	}
	if h.db != nil {
		if _, err := h.db.ExecContext(ctx, `CHECKPOINT`); err != nil {
			return errs.Wrap(err, "duckdb checkpoint")
		}
		if err := h.db.Close(); err != nil {
			return errs.Wrap(err, "close duckdb database")
		}
	}
	if h.cfg.Messages != nil {
		h.cfg.Log(2, "committed duckdb changes")
	}
	return nil
}

type collectionWriter struct {
	tx         *sql.Tx
	cfg        handlers.Config
	schema     string
	table      string
	cols       []string
	structType string
	upsert     bool
	pending    []string
}

func (w *collectionWriter) Write(row coll.Row) error {
	return w.append(row)
}

func (w *collectionWriter) Close() error {
	if err := w.flush(); err != nil {
		return err
	}

	colList := make([]string, len(w.cols))
	jsonCols := make([]string, len(w.cols))
	for i, c := range w.cols {
		colList[i] = quoteIdent(c)
		jsonCols[i] = "json." + quoteIdent(c)
	}

	kw := "INSERT INTO"
	if w.upsert {
		kw = "INSERT OR REPLACE INTO"
	}

	stmt := fmt.Sprintf(`%s %s.%s (%s) SELECT %s FROM (
		SELECT from_json(js.data, %s) AS json FROM __temp__json js
	)`,
		kw,
		quoteIdent(w.schema), quoteIdent(w.table),
		strings.Join(colList, ", "),
		strings.Join(jsonCols, ", "),
		quoteStructType(w.structType),
	)
	if w.cfg.Messages != nil {
		w.cfg.Log(3, stmt)
	}
	if _, err := w.tx.ExecContext(context.Background(), stmt); err != nil {
		return errs.Wrap(err, "insert from json", "table", w.table)
	}
	if _, err := w.tx.ExecContext(context.Background(), `DROP TABLE __temp__json`); err != nil {
		return errs.Wrap(err, "drop temp json table")
	}
	return nil
}

func (w *collectionWriter) append(row coll.Row) error {
	b, err := jsonx.Marshal(row)
	if err != nil {
		return errs.Wrap(err, "marshal row json")
	}
	w.pending = append(w.pending, string(b))
	if len(w.pending) >= jsonBatchSize {
		return w.flush()
	}
	return nil
}

func (w *collectionWriter) flush() error {
	if len(w.pending) == 0 {
		return nil
	}
	stmt, err := w.tx.PrepareContext(context.Background(), `INSERT INTO __temp__json (data) VALUES (?)`)
	if err != nil {
		return errs.Wrap(err, "prepare temp json insert")
	}
	defer stmt.Close()
	for _, js := range w.pending {
		if _, err := stmt.ExecContext(context.Background(), js); err != nil {
			return errs.Wrap(err, "insert temp json row")
		}
	}
	w.pending = w.pending[:0]
	return nil
}

func buildCreateTable(schema, table string, cols []string, sample coll.Row) string {
	parts := make([]string, len(cols))
	for i, c := range cols {
		parts[i] = fmt.Sprintf(`%s %s`, quoteIdent(c), inferColumnType(sample[c]))
	}
	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s.%s (%s)`,
		quoteIdent(schema), quoteIdent(table), strings.Join(parts, ", "))
}

func tableStructType(ctx context.Context, tx *sql.Tx, schema, table string) (string, error) {
	var structType string
	q := fmt.Sprintf(`
		SELECT json_group_object(column_name, data_type)::VARCHAR
		FROM information_schema.columns
		WHERE table_schema = '%s' AND table_name = '%s'`,
		escapeSQLString(schema), escapeSQLString(table))
	if err := tx.QueryRowContext(ctx, q).Scan(&structType); err != nil {
		return "", errs.Wrap(err, "read table struct type", "table", schema+"."+table)
	}
	return structType, nil
}

func quoteStructType(structType string) string {
	return fmt.Sprintf("$$%s$$", structType)
}

func escapeSQLString(s string) string {
	return strings.ReplaceAll(s, `'`, `''`)
}
