package mssql

import (
	"context"
	"database/sql"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/errs"
	"github.com/ceymard/swl-go/internal/handlers"
	"github.com/ceymard/swl-go/internal/ssh"
)

// Source reads tables or queries from Microsoft SQL Server.
type Source struct{}

func (Source) Source(ctx context.Context, cfg handlers.Config, raw any) (coll.Stream, error) {
	opts := raw.(SrcOpts)
	if opts.URI == "" {
		return nil, errs.New("mssql source requires a connection URI")
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
		SELECT TABLE_SCHEMA, TABLE_NAME
		FROM INFORMATION_SCHEMA.TABLES
		WHERE TABLE_TYPE = 'BASE TABLE'
		  AND TABLE_CATALOG = DB_NAME()
		ORDER BY TABLE_SCHEMA, TABLE_NAME`)
	if err != nil {
		return nil, errs.Wrap(err, "list mssql tables")
	}
	defer rows.Close()

	var specs []TableSpec
	for rows.Next() {
		var schema, name string
		if err := rows.Scan(&schema, &name); err != nil {
			return nil, errs.Wrap(err, "scan table name")
		}
		specs = append(specs, TableSpec{Name: schema + "." + name})
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
			cs := coll.NewColumnSet()
			c := coll.Collection{
				Name:    spec.Name,
				Columns: cs,
				Rows:    queryRows(ctx, db, cs, sqlText),
			}
			if !yield(c, nil) {
				return
			}
		}
	}
}

func queryRows(ctx context.Context, db *sql.DB, cs *coll.ColumnSet, query string) coll.RowBatches {
	return func(yield func([]coll.Row, error) bool) {
		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			yield(nil, errs.Wrap(err, "mssql query"))
			return
		}
		defer rows.Close()

		cols, err := rows.Columns()
		if err != nil {
			yield(nil, errs.Wrap(err, "mssql columns"))
			return
		}
		// rows.Columns() is already ordered; assign indexes 0..n-1 up front
		// so every row in this query shares the same positional layout.
		for _, name := range cols {
			cs.Index(name)
		}

		batch := make([]coll.Row, 0, coll.DefaultBatchSize)
		for rows.Next() {
			raw := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range raw {
				ptrs[i] = &raw[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				yield(nil, errs.Wrap(err, "mssql scan"))
				return
			}
			row := make(coll.Row, len(cols))
			for i := range cols {
				row[i] = normalizeCell(raw[i])
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
			yield(nil, errs.Wrap(err, "mssql rows"))
			return
		}
		if len(batch) > 0 {
			yield(batch, nil)
		}
	}
}
