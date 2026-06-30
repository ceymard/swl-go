package pg

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/errs"
	"github.com/ceymard/swl-go/internal/handlers"
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
		cfg.Messages.Log(1, "connected to postgres", opts.URI)
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
			sqlText := querySQL(spec)
			wrapped := fmt.Sprintf(`SELECT row_to_json(Q) AS row FROM (%s) Q`, sqlText)
			c := coll.Collection{
				Name: spec.Name,
				Rows: jsonRows(ctx, pool, wrapped),
			}
			if !yield(c, nil) {
				return
			}
		}
		if cfg.Messages != nil {
			cfg.Messages.Log(2, "finished sending postgres collections")
		}
	}
}

func jsonRows(ctx context.Context, pool interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, query string) iter.Seq2[coll.Row, error] {
	return func(yield func(coll.Row, error) bool) {
		rows, err := pool.Query(ctx, query)
		if err != nil {
			yield(nil, errs.Wrap(err, "postgres query"))
			return
		}
		defer rows.Close()

		for rows.Next() {
			var rowJSON []byte
			if err := rows.Scan(&rowJSON); err != nil {
				yield(nil, errs.Wrap(err, "postgres scan row"))
				return
			}
			var row map[string]any
			if err := json.Unmarshal(rowJSON, &row); err != nil {
				yield(nil, errs.Wrap(err, "postgres decode row json"))
				return
			}
			if !yield(row, nil) {
				return
			}
		}
		if err := rows.Err(); err != nil {
			yield(nil, errs.Wrap(err, "postgres rows"))
		}
	}
}
