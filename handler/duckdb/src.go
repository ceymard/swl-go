package duckdb

import (
	"context"
	"database/sql"
	"fmt"
	"iter"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/errs"
	"github.com/ceymard/swl-go/internal/handlers"
)

// Source reads tables or queries from a DuckDB database file (swl2 swl-duckdb-src.ts).
type Source struct{}

func (Source) Source(ctx context.Context, cfg handlers.Config, raw any) (coll.Stream, error) {
	opts := raw.(SrcOpts)
	if opts.File == "" {
		return nil, errs.New("duckdb source requires a database file path")
	}

	db, err := sql.Open(driverName, opts.File)
	if err != nil {
		return nil, errs.Wrap(err, "open duckdb database", "path", opts.File)
	}

	tables := opts.Tables
	if len(tables) == 0 {
		tables, err = listTables(ctx, db)
		if err != nil {
			_ = db.Close()
			return nil, err
		}
	}

	if cfg.Messages != nil {
		cfg.Messages.Log(2, "opened duckdb database", opts.File, "to read")
	}

	return streamTables(ctx, cfg, db, tables), nil
}

func listTables(ctx context.Context, db *sql.DB) ([]TableSpec, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT table_schema, table_name
		FROM information_schema.tables
		WHERE table_type = 'BASE TABLE'
		  AND table_schema NOT IN ('information_schema', 'pg_catalog')
		ORDER BY table_schema, table_name`)
	if err != nil {
		return nil, errs.Wrap(err, "list duckdb tables")
	}
	defer rows.Close()

	var specs []TableSpec
	for rows.Next() {
		var schema, name string
		if err := rows.Scan(&schema, &name); err != nil {
			return nil, errs.Wrap(err, "scan table name")
		}
		colName := name
		if schema != "main" {
			colName = schema + "." + name
		}
		specs = append(specs, TableSpec{Name: colName})
	}
	return specs, rows.Err()
}

func streamTables(ctx context.Context, cfg handlers.Config, db *sql.DB, tables []TableSpec) coll.Stream {
	return func(yield func(coll.Collection, error) bool) {
		defer db.Close()
		for _, spec := range tables {
			sqlText := spec.Query
			if sqlText == "" {
				sqlText = fmt.Sprintf("SELECT * FROM %s", spec.Name)
			}
			if cfg.Messages != nil {
				cfg.Messages.Log(3, jsonRowsSQL(sqlText))
			}
			c := coll.Collection{
				Name: spec.Name,
				Rows: queryJSONRows(ctx, db, sqlText),
			}
			if !yield(c, nil) {
				return
			}
		}
		if cfg.Messages != nil {
			cfg.Messages.Log(2, "finished sending duckdb collections")
		}
	}
}

func jsonRowsSQL(inner string) string {
	return fmt.Sprintf("SELECT to_json(J) AS json FROM (%s) J", inner)
}

func queryJSONRows(ctx context.Context, db *sql.DB, innerSQL string) iter.Seq2[coll.Row, error] {
	return func(yield func(coll.Row, error) bool) {
		rows, err := db.QueryContext(ctx, jsonRowsSQL(innerSQL))
		if err != nil {
			yield(nil, errs.Wrap(err, "duckdb query"))
			return
		}
		defer rows.Close()

		for rows.Next() {
			var cell any
			if err := rows.Scan(&cell); err != nil {
				yield(nil, errs.Wrap(err, "duckdb scan row"))
				return
			}
			row, err := rowFromJSONCell(cell)
			if err != nil {
				yield(nil, errs.Wrap(err, "duckdb parse row json"))
				return
			}
			if !yield(row, nil) {
				return
			}
		}
		if err := rows.Err(); err != nil {
			yield(nil, errs.Wrap(err, "duckdb rows"))
		}
	}
}
