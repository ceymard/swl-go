package pg

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/errs"
	"github.com/ceymard/swl-go/internal/handlers"
	"github.com/ceymard/swl-go/internal/jsonx"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Sink writes collections into PostgreSQL via COPY + json_populate_record (swl2 swl-pg-sink.ts).
type Sink struct{}

func (Sink) Sink(ctx context.Context, cfg handlers.Config, in coll.Stream, raw any) error {
	opts := raw.(SinkOpts)
	if opts.URI == "" {
		return errs.New("pg sink requires a postgres URI")
	}
	h := &sinkHooks{cfg: cfg, opts: opts}
	return handlers.ConsumeHooks(cfg, h, in)
}

type sinkHooks struct {
	cfg  handlers.Config
	opts SinkOpts
	pool *pgxpool.Pool
	tun  interface{ Close() error }
	tx   pgx.Tx
	seen map[string]bool
	seq  atomic.Uint64
}

func (h *sinkHooks) Init(ctx context.Context) error {
	pool, tun, err := connect(ctx, h.opts.URI)
	if err != nil {
		return err
	}
	h.pool = pool
	h.tun = tun
	h.seen = make(map[string]bool)

	tx, err := pool.Begin(ctx)
	if err != nil {
		_ = tun.Close()
		pool.Close()
		return errs.Wrap(err, "postgres begin")
	}
	h.tx = tx

	if h.opts.DisableTriggers {
		if h.cfg.Messages != nil {
			h.cfg.Messages.Log(1, "disabling triggers")
		}
		if _, err := tx.Exec(ctx, `SET session_replication_role = replica`); err != nil {
			return errs.Wrap(err, "disable triggers")
		}
	}
	if h.cfg.Messages != nil {
		h.cfg.Messages.Log(2, "connected to postgres", h.opts.URI)
	}
	return nil
}

func (h *sinkHooks) Open(ctx context.Context, col coll.Collection, firstRow coll.Row) (handlers.RowWriter, error) {
	colOpts := h.opts.Collections[col.Name]
	effective := mergeColOpts(h.opts, colOpts)

	schema, table := qualifyTable(col.Name, h.opts.Schema)
	fqTable := `"` + schema + `"."` + table + `"`

	if !h.seen[col.Name] {
		if effective.Drop {
			if h.cfg.Messages != nil {
				h.cfg.Messages.Log(1, "dropping", col.Name)
			}
			if _, err := h.tx.Exec(ctx, `DROP TABLE IF EXISTS `+fqTable); err != nil {
				return nil, errs.Wrap(err, "drop table", "table", col.Name)
			}
		}
		cols := columnNames(firstRow)
		if effective.AutoCreate {
			if h.cfg.Messages != nil {
				h.cfg.Messages.Log(1, "creating", col.Name)
			}
			if _, err := h.tx.Exec(ctx, buildCreateTable(fqTable, cols)); err != nil {
				return nil, errs.Wrap(err, "create table", "table", col.Name)
			}
		}
		if effective.Truncate {
			if h.cfg.Messages != nil {
				h.cfg.Messages.Log(1, "truncating", col.Name)
			}
			if _, err := h.tx.Exec(ctx, `TRUNCATE `+fqTable+` RESTART IDENTITY CASCADE`); err != nil {
				return nil, errs.Wrap(err, "truncate table", "table", col.Name)
			}
		}
		h.seen[col.Name] = true
	}

	cols := columnNames(firstRow)
	tempTable := fmt.Sprintf("swl_temp_%d", h.seq.Add(1))
	if _, err := h.tx.Exec(ctx, fmt.Sprintf(`CREATE TEMP TABLE %s (jsondata json) ON COMMIT DROP`, tempTable)); err != nil {
		return nil, errs.Wrap(err, "create temp copy table", "table", col.Name)
	}

	pr, pw := io.Pipe()
	copySQL := fmt.Sprintf(`COPY %s(jsondata) FROM STDIN WITH (NULL '**NULL**', DELIMITER '|', FORMAT csv, QUOTE '@')`, tempTable)
	copyDone := make(chan error, 1)
	go func() {
		pgConn := h.tx.Conn().PgConn()
		_, err := pgConn.CopyFrom(ctx, pr, copySQL)
		copyDone <- err
	}()

	w := &copyWriter{
		ctx:       ctx,
		tx:        h.tx,
		cfg:       h.cfg,
		cols:      cols,
		fqTable:   fqTable,
		table:     col.Name,
		tempTable: tempTable,
		colOpts:   effective,
		global:    h.opts,
		pw:        pw,
		copyDone:  copyDone,
	}
	if err := w.writeRow(firstRow); err != nil {
		_ = pw.Close()
		<-copyDone
		return nil, err
	}
	return w, nil
}

func (h *sinkHooks) Rollback(ctx context.Context) {
	if h.tx != nil {
		_ = h.tx.Rollback(ctx)
	}
	if h.pool != nil {
		h.pool.Close()
	}
	if h.tun != nil {
		_ = h.tun.Close()
	}
	if h.cfg.Messages != nil {
		h.cfg.Messages.Log(2, "rolled back postgres transaction")
	}
}

func (h *sinkHooks) Finish(ctx context.Context) error {
	if h.tx != nil {
		if err := h.tx.Commit(ctx); err != nil {
			return errs.Wrap(err, "postgres commit")
		}
	}
	if h.pool != nil {
		h.pool.Close()
	}
	if h.tun != nil {
		_ = h.tun.Close()
	}
	if h.cfg.Messages != nil {
		h.cfg.Messages.Log(2, "committed postgres changes")
	}
	return nil
}

type copyWriter struct {
	ctx       context.Context
	tx        pgx.Tx
	cfg       handlers.Config
	cols      []string
	fqTable   string
	table     string
	tempTable string
	colOpts   colSinkOpts
	global    SinkOpts
	pw        *io.PipeWriter
	copyDone  chan error
	closed    bool
}

func (w *copyWriter) Write(row coll.Row) error {
	if w.closed {
		return errs.New("copy writer closed")
	}
	return w.writeRow(row)
}

func (w *copyWriter) Close() error {
	if w.closed {
		return nil
	}
	w.closed = true
	if w.pw != nil {
		if err := w.pw.Close(); err != nil {
			return errs.Wrap(err, "close copy stream", "table", w.table)
		}
		if err := <-w.copyDone; err != nil {
			return errs.Wrap(err, "postgres copy", "table", w.table)
		}
	}
	if err := w.flushToTable(); err != nil {
		return err
	}
	if w.cfg.Messages != nil && w.cfg.Verbose >= 2 {
		schema, table := qualifyTable(w.table, w.global.Schema)
		var n int64
		q := fmt.Sprintf(`SELECT count(*) FROM "%s"."%s"`, schema, table)
		if err := w.tx.QueryRow(w.ctx, q).Scan(&n); err == nil {
			w.cfg.Messages.Log(2, "table", w.table, "now has", n, "rows")
		}
	}
	return nil
}

func (w *copyWriter) writeRow(row coll.Row) error {
	payload, err := copyLine(row)
	if err != nil {
		return err
	}
	if _, err := w.pw.Write(payload); err != nil {
		return errs.Wrap(err, "copy row", "table", w.table)
	}
	return nil
}

func copyLine(row coll.Row) ([]byte, error) {
	data, err := jsonx.Marshal(row)
	if err != nil {
		return nil, err
	}
	s := string(data)
	s = strings.ReplaceAll(s, "@", "@@")
	return []byte("@" + s + "@\n"), nil
}

func (w *copyWriter) flushToTable() error {
	if w.colOpts.Update || w.global.Update {
		return w.runUpdate()
	}

	quoted := quoteColumns(w.cols)
	selectCols := make([]string, len(w.cols))
	for i, c := range w.cols {
		selectCols[i] = `R."` + c + `"`
	}

	upsert, err := w.buildUpsertClause()
	if err != nil {
		return err
	}

	sql := fmt.Sprintf(`
		INSERT INTO %s (%s)
		SELECT %s
		FROM %s T,
			json_populate_record(null::%s, T.jsondata) R
		%s`, w.fqTable, strings.Join(quoted, ", "), strings.Join(selectCols, ", "),
		w.tempTable, w.fqTable, upsert)

	if _, err := w.tx.Exec(w.ctx, sql); err != nil {
		return errs.Wrap(err, "insert from copy temp", "table", w.table)
	}
	return nil
}

func (w *copyWriter) runUpdate() error {
	rows, err := w.tx.Query(w.ctx, `
		SELECT a.attname
		FROM pg_index i
		JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = ANY(i.indkey)
		WHERE i.indrelid = $1::regclass AND i.indisprimary
		ORDER BY a.attnum`, w.fqTable)
	if err != nil {
		return errs.Wrap(err, "primary key for update mode", "table", w.table)
	}
	defer rows.Close()
	var pkCols []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return errs.Wrap(err, "scan pk column")
		}
		pkCols = append(pkCols, name)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(pkCols) == 0 {
		return errs.New("update mode requires a primary key on " + w.table)
	}

	setParts := make([]string, 0, len(w.cols))
	for _, c := range w.cols {
		if containsString(pkCols, c) {
			continue
		}
		setParts = append(setParts, fmt.Sprintf(`"%s" = (T.rec)."%s"`, c, c))
	}
	whereParts := make([]string, len(pkCols))
	for i, c := range pkCols {
		whereParts[i] = fmt.Sprintf(`(T.rec)."%s" = %s."%s"`, c, w.fqTable, c)
	}

	sql := fmt.Sprintf(`
		UPDATE %s
		SET %s
		FROM (
			SELECT json_populate_record(null::%s, T.jsondata) AS rec
			FROM %s T
		) T
		WHERE %s`, w.fqTable, strings.Join(setParts, ", "), w.fqTable, w.tempTable, strings.Join(whereParts, " AND "))

	if _, err := w.tx.Exec(w.ctx, sql); err != nil {
		return errs.Wrap(err, "update from copy temp", "table", w.table)
	}
	return nil
}

func (w *copyWriter) buildUpsertClause() (string, error) {
	if !w.colOpts.Upsert && !w.global.Upsert && !w.colOpts.DoNothing && !w.global.DoNothing {
		return "", nil
	}
	schema, table := parseFQTable(w.fqTable)
	var constraint string
	err := w.tx.QueryRow(w.ctx, `
		SELECT constraint_name
		FROM information_schema.table_constraints
		WHERE table_schema = $1 AND table_name = $2
		  AND constraint_type IN ('PRIMARY KEY', 'UNIQUE')
		ORDER BY CASE constraint_type WHEN 'PRIMARY KEY' THEN 0 ELSE 1 END
		LIMIT 1`, schema, table).Scan(&constraint)
	if err != nil {
		return "", nil
	}
	if w.colOpts.DoNothing || w.global.DoNothing {
		return fmt.Sprintf(` ON CONFLICT ON CONSTRAINT "%s" DO NOTHING`, constraint), nil
	}
	setParts := make([]string, len(w.cols))
	for i, c := range w.cols {
		setParts[i] = fmt.Sprintf(`"%s" = EXCLUDED."%s"`, c, c)
	}
	return fmt.Sprintf(` ON CONFLICT ON CONSTRAINT "%s" DO UPDATE SET %s`,
		constraint, strings.Join(setParts, ", ")), nil
}

func mergeColOpts(global SinkOpts, col colSinkOpts) colSinkOpts {
	out := col
	if global.Truncate {
		out.Truncate = true
	}
	if global.Drop {
		out.Drop = true
	}
	if global.Upsert {
		out.Upsert = true
	}
	if global.Update {
		out.Update = true
	}
	if global.AutoCreate {
		out.AutoCreate = true
	}
	if global.DoNothing {
		out.DoNothing = true
	}
	return out
}

func buildCreateTable(fqTable string, cols []string) string {
	parts := make([]string, len(cols))
	for i, c := range cols {
		parts[i] = fmt.Sprintf(`"%s" text`, c)
	}
	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (%s)`, fqTable, strings.Join(parts, ", "))
}

func columnNames(row coll.Row) []string {
	names := make([]string, 0, len(row))
	for k := range row {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

func quoteColumns(cols []string) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = `"` + c + `"`
	}
	return out
}

func containsString(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func parseFQTable(fqTable string) (schema, table string) {
	clean := strings.ReplaceAll(fqTable, `"`, "")
	parts := strings.Split(clean, ".")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "public", clean
}
