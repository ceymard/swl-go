package pg

import (
	"context"
	"fmt"
	"io"
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
		if effective.AutoCreate {
			if h.cfg.Messages != nil {
				h.cfg.Messages.Log(1, "creating", col.Name)
			}
			if _, err := h.tx.Exec(ctx, buildCreateTable(fqTable, columnNames(firstRow))); err != nil {
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

	if h.opts.IgnoreNonExisting {
		var exists bool
		err := h.tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM information_schema.tables
				WHERE table_schema = $1 AND table_name = $2
			)`, schema, table).Scan(&exists)
		if err != nil {
			return nil, errs.Wrap(err, "check table exists", "table", col.Name)
		}
		if !exists {
			if h.cfg.Messages != nil {
				h.cfg.Messages.Log(1, "ignoring non-existing table", col.Name)
			}
			return &noopWriter{}, nil
		}
	}

	hstoreCols, err := listHstoreColumns(ctx, h.tx, schema, table)
	if err != nil {
		return nil, err
	}

	cols := columnNames(firstRow)
	tempTable := tempTableName(col.Name)
	if _, err := h.tx.Exec(ctx, fmt.Sprintf(`CREATE TEMP TABLE %s (jsondata json)`, tempTable)); err != nil {
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

	droppedIndexes, err := droppedIndexesForTable(ctx, h.tx, h.cfg, effective, schema, table)
	if err != nil {
		_ = pw.Close()
		<-copyDone
		return nil, err
	}

	w := &copyWriter{
		ctx:            ctx,
		tx:             h.tx,
		cfg:            h.cfg,
		cols:           cols,
		fqTable:        fqTable,
		rowType:        tableRowTypeRef(col.Name, h.opts.Schema),
		table:          col.Name,
		schema:         schema,
		tableName:      table,
		tempTable:      tempTable,
		colOpts:        effective,
		global:         h.opts,
		hstoreCols:     hstoreCols,
		droppedIndexes: droppedIndexes,
		pw:             pw,
		copyDone:       copyDone,
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
		if _, err := h.pool.Exec(ctx, `ANALYZE`); err != nil {
			h.pool.Close()
			if h.tun != nil {
				_ = h.tun.Close()
			}
			return errs.Wrap(err, "postgres analyze")
		}
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

type noopWriter struct{}

func (noopWriter) Write(coll.Row) error { return nil }
func (noopWriter) Close() error         { return nil }

type copyWriter struct {
	ctx            context.Context
	tx             pgx.Tx
	cfg            handlers.Config
	cols           []string
	fqTable        string
	rowType        string
	table          string
	schema         string
	tableName      string
	tempTable      string
	colOpts        colSinkOpts
	global         SinkOpts
	hstoreCols     []string
	droppedIndexes []pgIndex
	pw             *io.PipeWriter
	copyDone       chan error
	closed         bool
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
	if _, err := w.tx.Exec(w.ctx, fmt.Sprintf(`DROP TABLE %s`, w.tempTable)); err != nil {
		return errs.Wrap(err, "drop temp copy table", "table", w.table)
	}
	if err := recreateIndexes(w.ctx, w.tx, w.cfg, w.droppedIndexes); err != nil {
		return err
	}
	if err := resetTableSequences(w.ctx, w.tx, w.cfg, w.table, w.schema, w.tableName); err != nil {
		return err
	}
	if w.cfg.Messages != nil && w.cfg.Verbose >= 2 {
		var n int64
		q := fmt.Sprintf(`SELECT count(*) FROM %s`, w.fqTable)
		if err := w.tx.QueryRow(w.ctx, q).Scan(&n); err == nil {
			w.cfg.Messages.Log(2, "table", w.table, "now has", n, "rows")
		}
	}
	return nil
}

func (w *copyWriter) writeRow(row coll.Row) error {
	row = transformHstoreColumns(row, w.hstoreCols)
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
		WITH upsert AS (
			INSERT INTO %s (%s)
			SELECT %s
			FROM %s T,
				json_populate_record(null::%s, T.jsondata) R
			%s
			RETURNING (xmax = 0) AS inserted
		)
		SELECT
			COUNT(*) FILTER (WHERE inserted) AS num_inserted,
			COUNT(*) FILTER (WHERE NOT inserted) AS num_updated
		FROM upsert`,
		w.fqTable, strings.Join(quoted, ", "), strings.Join(selectCols, ", "),
		w.tempTable, w.rowType, upsert)

	if w.cfg.Messages != nil {
		w.cfg.Messages.Log(3, sql)
	}

	var inserted, updated int64
	if err := w.tx.QueryRow(w.ctx, sql).Scan(&inserted, &updated); err != nil {
		return errs.Wrap(err, "insert from copy temp", "table", w.table)
	}
	if w.cfg.Messages != nil {
		if w.colOpts.Upsert || w.global.Upsert {
			w.cfg.Messages.Log(1, w.table, inserted, "rows inserted,", updated, "rows updated")
		} else {
			w.cfg.Messages.Log(1, w.table, inserted, "rows inserted")
		}
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
		WHERE %s`, w.fqTable, strings.Join(setParts, ", "), w.rowType, w.tempTable, strings.Join(whereParts, " AND "))

	if w.cfg.Messages != nil {
		w.cfg.Messages.Log(3, sql)
	}
	tag, err := w.tx.Exec(w.ctx, sql)
	if err != nil {
		return errs.Wrap(err, "update from copy temp", "table", w.table)
	}
	if w.cfg.Messages != nil {
		w.cfg.Messages.Log(1, w.table, tag.RowsAffected(), "rows updated")
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

func listHstoreColumns(ctx context.Context, tx pgx.Tx, schema, table string) ([]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT column_name
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2 AND udt_name = 'hstore'
		ORDER BY ordinal_position`, schema, table)
	if err != nil {
		return nil, errs.Wrap(err, "list hstore columns", "table", schema+"."+table)
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, errs.Wrap(err, "scan hstore column")
		}
		cols = append(cols, name)
	}
	return cols, rows.Err()
}

func droppedIndexesForTable(ctx context.Context, tx pgx.Tx, cfg handlers.Config, opts colSinkOpts, schema, table string) ([]pgIndex, error) {
	if !opts.DropIndexes {
		return nil, nil
	}
	return dropTableIndexes(ctx, tx, cfg, schema, table)
}

func dropTableIndexes(ctx context.Context, tx pgx.Tx, cfg handlers.Config, schema, table string) ([]pgIndex, error) {
	rows, err := tx.Query(ctx, `
		SELECT schemaname, indexname, indexdef
		FROM pg_indexes
		WHERE tablename = $1 AND schemaname = $2`, table, schema)
	if err != nil {
		return nil, errs.Wrap(err, "list indexes", "table", schema+"."+table)
	}
	defer rows.Close()
	var indices []pgIndex
	for rows.Next() {
		var idx pgIndex
		if err := rows.Scan(&idx.Schema, &idx.Name, &idx.Def); err != nil {
			return nil, errs.Wrap(err, "scan index")
		}
		indices = append(indices, idx)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, idx := range indices {
		if cfg.Messages != nil {
			cfg.Messages.Log(1, "dropping index", idx.Name)
		}
		stmt := fmt.Sprintf(`DROP INDEX "%s"."%s"`, idx.Schema, idx.Name)
		if _, err := tx.Exec(ctx, stmt); err != nil {
			return nil, errs.Wrap(err, "drop index", "index", idx.Name)
		}
	}
	return indices, nil
}

func recreateIndexes(ctx context.Context, tx pgx.Tx, cfg handlers.Config, indices []pgIndex) error {
	for _, idx := range indices {
		if cfg.Messages != nil {
			cfg.Messages.Log(1, "recreating index", idx.Name)
		}
		if _, err := tx.Exec(ctx, idx.Def); err != nil {
			return errs.Wrap(err, "recreate index", "index", idx.Name)
		}
	}
	return nil
}

func resetTableSequences(ctx context.Context, tx pgx.Tx, cfg handlers.Config, fqCollection, schema, table string) error {
	rows, err := tx.Query(ctx, `
		SELECT column_name,
			CASE WHEN is_identity = 'YES' THEN pg_get_serial_sequence(format('%I.%I', $1::text, $2::text), column_name)
				ELSE regexp_replace(regexp_replace(column_default, '[^'']+''', ''), '''.*', '')
			END AS seq
		FROM information_schema.columns
		WHERE table_schema = $1 AND table_name = $2
		  AND (column_default LIKE '%nextval(%' OR is_identity = 'YES')`, schema, table)
	if err != nil {
		return errs.Wrap(err, "list sequences", "table", fqCollection)
	}
	defer rows.Close()

	type seqInfo struct {
		column string
		seq    string
	}
	var seqs []seqInfo
	for rows.Next() {
		var s seqInfo
		if err := rows.Scan(&s.column, &s.seq); err != nil {
			return errs.Wrap(err, "scan sequence")
		}
		if s.seq != "" {
			seqs = append(seqs, s)
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	for _, s := range seqs {
		if cfg.Messages != nil {
			cfg.Messages.Log(2, "resetting sequence", s.seq)
		}
		sql := fmt.Sprintf(`
			DO $$
			DECLARE
				themax INT;
			BEGIN
				LOCK TABLE %s IN EXCLUSIVE MODE;
				SELECT MAX("%s") INTO themax FROM %s;
				PERFORM setval('%s', COALESCE(themax + 1, 1), false);
			END
			$$ LANGUAGE plpgsql`, `"`+schema+`"."`+table+`"`, s.column, `"`+schema+`"."`+table+`"`, s.seq)
		if _, err := tx.Exec(ctx, sql); err != nil {
			return errs.Wrap(err, "reset sequence", "sequence", s.seq)
		}
	}
	return nil
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
	if global.DropIndexes {
		out.DropIndexes = true
	}
	return out
}
