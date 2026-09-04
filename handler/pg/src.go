package pg

import (
	"context"
	"time"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/errs"
	"github.com/ceymard/swl-go/internal/handlers"
	"github.com/ceymard/swl-go/internal/jsonx"
	"github.com/jackc/pgx/v5"
)

// Source reads tables or queries from PostgreSQL (swl2 swl-pg-src.ts).
type Source struct{}

func (Source) Source(ctx context.Context, cfg handlers.Config, raw any) (coll.Stream, error) {
	opts := raw.(SrcOpts)
	if opts.URI == "" {
		return nil, errs.New("pg source requires a postgres URI")
	}

	pool, tun, err := connect(ctx, opts.URI)
	if err != nil {
		return nil, err
	}

	sources := opts.Sources
	if len(sources) == 0 {
		sources, err = listSchemaTables(ctx, pool, opts.Schema)
		if err != nil {
			pool.Close()
			_ = tun.Close()
			return nil, err
		}
	} else {
		sources, err = expandWildcardSources(ctx, pool, sources)
		if err != nil {
			pool.Close()
			_ = tun.Close()
			return nil, err
		}
	}

	if cfg.Messages != nil {
		cfg.Log(1, "connected to", cfg.ConnTarget(opts.URI))
	}

	return streamQueries(ctx, cfg, pool, tun, sources), nil
}

func streamQueries(ctx context.Context, cfg handlers.Config, pool interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	Close()
}, tun interface{ Close() error }, sources []TableSpec) coll.Stream {
	return func(yield func(coll.Collection, error) bool) {
		defer pool.Close()
		defer tun.Close()

		for _, spec := range sources {
			cs := coll.NewColumnSet()
			c := coll.Collection{
				Name:    spec.Name,
				Columns: cs,
				Rows:    queryRows(ctx, pool, cs, querySQL(spec)),
			}
			if !yield(c, nil) {
				return
			}
		}
	}
}

func queryRows(ctx context.Context, pool interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, cs *coll.ColumnSet, query string) coll.RowBatches {
	return func(yield func([]coll.Row, error) bool) {
		rows, err := pool.Query(ctx, query)
		if err != nil {
			yield(nil, errs.Wrap(err, "postgres query"))
			return
		}
		defer rows.Close()

		// FieldDescriptions is ordered (unlike pgx.RowToMap, which builds a
		// Go map and discards column order); assign indexes up front so
		// every row in this query shares the same positional layout.
		for _, fd := range rows.FieldDescriptions() {
			cs.Index(fd.Name)
		}

		batch := make([]coll.Row, 0, coll.DefaultBatchSize)
		for rows.Next() {
			vals, err := rows.Values()
			if err != nil {
				yield(nil, errs.Wrap(err, "postgres scan row"))
				return
			}
			row := make(coll.Row, len(vals))
			for i, v := range vals {
				row[i] = normalizePGCell(v)
			}
			batch = append(batch, row)
			if len(batch) == coll.DefaultBatchSize {
				if !yield(batch, nil) {
					return
				}
				batch = make([]coll.Row, 0, coll.DefaultBatchSize)
			}
		}
		if err := rows.Err(); err != nil {
			yield(nil, errs.Wrap(err, "postgres rows"))
			return
		}
		if len(batch) > 0 {
			yield(batch, nil)
		}
	}
}

// normalizePGCell maps pgx driver values to coll.Row cells (native ints, times, …).
func normalizePGCell(v any) any {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case []byte:
		return maybeParseJSON(string(x))
	case time.Time:
		return x.UTC()
	default:
		return v
	}
}

func maybeParseJSON(v any) any {
	s, ok := v.(string)
	if !ok || len(s) == 0 {
		return v
	}
	if s[0] != '{' && s[0] != '[' {
		return v
	}
	var out any
	if err := jsonx.Unmarshal([]byte(s), &out); err != nil {
		return v
	}
	return out
}
