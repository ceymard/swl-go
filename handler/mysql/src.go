package mysql

import (
	"context"
	"database/sql"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/errs"
	"github.com/ceymard/swl-go/internal/handlers"
	"github.com/ceymard/swl-go/internal/ssh"
)

// Source reads tables or queries from MySQL (swl2 swl-my-src.ts).
type Source struct{}

func (Source) Source(ctx context.Context, cfg handlers.Config, raw any) (coll.Stream, error) {
	opts := raw.(SrcOpts)
	if opts.URI == "" {
		return nil, errs.New("mysql source requires a connection URI")
	}

	db, tun, err := connect(ctx, opts.URI)
	if err != nil {
		return nil, err
	}

	sources := opts.Sources
	if len(sources) == 0 {
		sources, err = listTables(ctx, db)
		if err != nil {
			_ = db.Close()
			_ = tun.Close()
			return nil, err
		}
	}

	if cfg.Messages != nil {
		cfg.Log(1, "connected to", cfg.ConnTarget(opts.URI))
	}

	return streamTables(ctx, cfg, db, tun, sources), nil
}

func listTables(ctx context.Context, db *sql.DB) ([]TableSpec, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = DATABASE()
		  AND table_type = 'BASE TABLE'
		ORDER BY table_name`)
	if err != nil {
		return nil, errs.Wrap(err, "list mysql tables")
	}
	defer rows.Close()

	var specs []TableSpec
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, errs.Wrap(err, "scan table name")
		}
		specs = append(specs, TableSpec{Name: name})
	}
	return specs, rows.Err()
}

func streamTables(ctx context.Context, cfg handlers.Config, db *sql.DB, tun *ssh.OpenResult, sources []TableSpec) coll.Stream {
	return func(yield func(coll.Collection, error) bool) {
		defer db.Close()
		defer tun.Close()
		for _, spec := range sources {
			sqlText := querySQL(spec)
			if cfg.Messages != nil {
				cfg.Log(3, sqlText)
			}
			c := coll.Collection{
				Name: spec.Name,
				Rows: queryRows(ctx, db, sqlText),
			}
			if !yield(c, nil) {
				return
			}
		}
	}
}

func queryRows(ctx context.Context, db *sql.DB, query string) coll.RowBatches {
	return func(yield func([]coll.Row, error) bool) {
		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			yield(nil, errs.Wrap(err, "mysql query"))
			return
		}
		defer rows.Close()

		cols, err := rows.Columns()
		if err != nil {
			yield(nil, errs.Wrap(err, "mysql columns"))
			return
		}

		batch := make([]coll.Row, 0, coll.DefaultBatchSize)
		for rows.Next() {
			raw := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range raw {
				ptrs[i] = &raw[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				yield(nil, errs.Wrap(err, "mysql scan"))
				return
			}
			row := make(coll.Row, len(cols))
			for i, name := range cols {
				row[name] = normalizeCell(raw[i])
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
			yield(nil, errs.Wrap(err, "mysql rows"))
			return
		}
		if len(batch) > 0 {
			yield(batch, nil)
		}
	}
}
