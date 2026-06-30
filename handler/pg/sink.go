package pg

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/errs"
	"github.com/ceymard/swl-go/internal/handlers"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Sink writes collections into PostgreSQL (swl2 swl-pg-sink.ts, simplified INSERT path).
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
	cfg   handlers.Config
	opts  SinkOpts
	pool  *pgxpool.Pool
	tun   interface{ Close() error }
	tx    pgx.Tx
	seen  map[string]bool
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
			ddl := buildCreateTable(fqTable, cols)
			if _, err := h.tx.Exec(ctx, ddl); err != nil {
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
	insertSQL, err := buildInsert(ctx, h.tx, fqTable, cols, effective, h.opts)
	if err != nil {
		return nil, err
	}

	w := &rowWriter{
		tx:    h.tx,
		sql:   insertSQL,
		cols:  cols,
		table: col.Name,
		cfg:   h.cfg,
	}
	if err := w.insert(ctx, firstRow); err != nil {
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

type rowWriter struct {
	tx    pgx.Tx
	sql   string
	cols  []string
	table string
	cfg   handlers.Config
}

func (w *rowWriter) Write(row coll.Row) error {
	return w.insert(context.Background(), row)
}

func (w *rowWriter) Close() error {
	if w.cfg.Messages != nil && w.cfg.Verbose >= 2 {
		var n int64
		schema, table := qualifyTable(w.table, "public")
		q := fmt.Sprintf(`SELECT count(*) FROM "%s"."%s"`, schema, table)
		if err := w.tx.QueryRow(context.Background(), q).Scan(&n); err == nil {
			w.cfg.Messages.Log(2, "table", w.table, "now has", n, "rows")
		}
	}
	return nil
}

func (w *rowWriter) insert(ctx context.Context, row coll.Row) error {
	args := make([]any, len(w.cols))
	for i, c := range w.cols {
		v, err := bindValue(row[c])
		if err != nil {
			return errs.Wrap(err, "bind value", "column", c)
		}
		args[i] = v
	}
	if _, err := w.tx.Exec(ctx, w.sql, args...); err != nil {
		return errs.Wrap(err, "insert row", "table", w.table)
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
	return out
}

func buildCreateTable(fqTable string, cols []string) string {
	parts := make([]string, len(cols))
	for i, c := range cols {
		parts[i] = fmt.Sprintf(`"%s" text`, c)
	}
	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (%s)`, fqTable, strings.Join(parts, ", "))
}

func buildInsert(ctx context.Context, tx pgx.Tx, fqTable string, cols []string, col colSinkOpts, global SinkOpts) (string, error) {
	quoted := make([]string, len(cols))
	placeholders := make([]string, len(cols))
	for i, c := range cols {
		quoted[i] = `"` + c + `"`
		placeholders[i] = fmt.Sprintf("$%d", i+1)
	}
	sql := fmt.Sprintf(`INSERT INTO %s (%s) VALUES (%s)`,
		fqTable, strings.Join(quoted, ", "), strings.Join(placeholders, ", "))

	if !col.Upsert && !global.Upsert && !col.DoNothing && !global.DoNothing {
		return sql, nil
	}

	schema, table := parseFQTable(fqTable)

	var constraint string
	err := tx.QueryRow(ctx, `
		SELECT constraint_name
		FROM information_schema.table_constraints
		WHERE table_schema = $1 AND table_name = $2
		  AND constraint_type IN ('PRIMARY KEY', 'UNIQUE')
		ORDER BY CASE constraint_type WHEN 'PRIMARY KEY' THEN 0 ELSE 1 END
		LIMIT 1`, schema, table).Scan(&constraint)
	if err != nil {
		return sql, nil // plain insert when no constraint
	}

	if col.DoNothing || global.DoNothing {
		return sql + fmt.Sprintf(` ON CONFLICT ON CONSTRAINT "%s" DO NOTHING`, constraint), nil
	}

	setParts := make([]string, len(cols))
	for i, c := range cols {
		setParts[i] = fmt.Sprintf(`"%s" = EXCLUDED."%s"`, c, c)
	}
	return sql + fmt.Sprintf(` ON CONFLICT ON CONSTRAINT "%s" DO UPDATE SET %s`,
		constraint, strings.Join(setParts, ", ")), nil
}

func columnNames(row coll.Row) []string {
	names := make([]string, 0, len(row))
	for k := range row {
		names = append(names, k)
	}
	// stable order
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			if names[j] < names[i] {
				names[i], names[j] = names[j], names[i]
			}
		}
	}
	return names
}

func bindValue(v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	switch x := v.(type) {
	case string:
		return x, nil
	case map[string]any, []any:
		b, err := json.Marshal(x)
		if err != nil {
			return nil, err
		}
		return string(b), nil
	case int:
		return strconv.Itoa(x), nil
	case int32:
		return strconv.FormatInt(int64(x), 10), nil
	case int64:
		return strconv.FormatInt(x, 10), nil
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10), nil
		}
		return strconv.FormatFloat(x, 'f', -1, 64), nil
	case bool:
		return strconv.FormatBool(x), nil
	default:
		return fmt.Sprint(x), nil
	}
}

func parseFQTable(fqTable string) (schema, table string) {
	clean := strings.ReplaceAll(fqTable, `"`, "")
	parts := strings.Split(clean, ".")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "public", clean
}
