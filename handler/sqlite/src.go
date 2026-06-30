package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"iter"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/errs"
	"github.com/ceymard/swl-go/internal/handlers"
)

// Source reads tables or queries from a SQLite database file.
type Source struct{}

func (Source) Source(ctx context.Context, cfg handlers.Config, raw any) (coll.Stream, error) {
	opts := raw.(SrcOpts)
	if opts.File == "" {
		return nil, errs.New("sqlite source requires a database file path")
	}

	db, err := sql.Open(driverName, dsnReadOnly(opts.File))
	if err != nil {
		return nil, errs.Wrap(err, "open sqlite database", "path", opts.File)
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
		cfg.Messages.Log(2, "opened sqlite database", opts.File, "to read")
	}

	return streamTables(ctx, cfg, db, tables), nil
}

func listTables(ctx context.Context, db *sql.DB) ([]TableSpec, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT name FROM sqlite_master
		WHERE type='table' AND name NOT LIKE 'sqlite_stat%'
		ORDER BY name`)
	if err != nil {
		return nil, errs.Wrap(err, "list sqlite tables")
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

func streamTables(ctx context.Context, cfg handlers.Config, db *sql.DB, tables []TableSpec) coll.Stream {
	return func(yield func(coll.Collection, error) bool) {
		defer db.Close()
		for _, spec := range tables {
			sqlText := spec.Query
			if sqlText == "" {
				sqlText = fmt.Sprintf(`SELECT * FROM "%s"`, spec.Name)
			}
			c := coll.Collection{
				Name: spec.Name,
				Rows: queryRows(ctx, db, sqlText),
			}
			if !yield(c, nil) {
				return
			}
		}
		if cfg.Messages != nil {
			cfg.Messages.Log(2, "finished sending sqlite collections")
		}
	}
}

func queryRows(ctx context.Context, db *sql.DB, query string) iter.Seq2[coll.Row, error] {
	return func(yield func(coll.Row, error) bool) {
		rows, err := db.QueryContext(ctx, query)
		if err != nil {
			yield(nil, errs.Wrap(err, "sqlite query"))
			return
		}
		defer rows.Close()

		cols, err := rows.Columns()
		if err != nil {
			yield(nil, errs.Wrap(err, "sqlite columns"))
			return
		}

		for rows.Next() {
			raw := make([]any, len(cols))
			ptrs := make([]any, len(cols))
			for i := range raw {
				ptrs[i] = &raw[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				yield(nil, errs.Wrap(err, "sqlite scan"))
				return
			}
			row := make(coll.Row, len(cols))
			for i, name := range cols {
				row[name] = normalizeScan(raw[i])
			}
			if !yield(row, nil) {
				return
			}
		}
		if err := rows.Err(); err != nil {
			yield(nil, errs.Wrap(err, "sqlite rows"))
		}
	}
}

func normalizeScan(v any) any {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	v = maybeParseJSON(v)
	return v
}
