package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

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
		cfg.Log(2, "opened sqlite database", cfg.ConnTarget(opts.File), "to read")
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
				sqlText = fmt.Sprintf(`SELECT * FROM "%s"`, strings.ReplaceAll(spec.Name, `"`, `""`))
			}
			c := coll.Collection{
				Name: spec.Name,
				Rows: queryRows(ctx, db, sqlText, spec.Name),
			}
			if !yield(c, nil) {
				return
			}
		}
	}
}

func queryRows(ctx context.Context, db *sql.DB, query, tableName string) coll.RowBatches {
	return func(yield func([]coll.Row, error) bool) {
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

		colTypes, err := rows.ColumnTypes()
		if err != nil {
			yield(nil, errs.Wrap(err, "sqlite column types"))
			return
		}

		declTypes, err := declaredColumnTypes(ctx, db, tableName, cols, colTypes)
		if err != nil {
			yield(nil, err)
			return
		}
		jsonCols := jsonColumnsFromDeclared(cols, declTypes)

		batch := make([]coll.Row, 0, coll.DefaultBatchSize)
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
				row[name] = normalizeScanCell(raw[i], jsonCols[name])
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
			yield(nil, errs.Wrap(err, "sqlite rows"))
			return
		}
		if len(batch) > 0 {
			yield(batch, nil)
		}
	}
}

func declaredColumnTypes(ctx context.Context, db *sql.DB, table string, cols []string, colTypes []*sql.ColumnType) (map[string]string, error) {
	out := make(map[string]string, len(cols))
	for i, name := range cols {
		if i < len(colTypes) && colTypes[i] != nil {
			if t := colTypes[i].DatabaseTypeName(); t != "" {
				out[name] = t
			}
		}
	}
	if table != "" {
		pragmaTypes, err := columnTypesFromTableInfo(ctx, db, table)
		if err != nil {
			return nil, err
		}
		for _, name := range cols {
			if out[name] == "" {
				if t, ok := pragmaTypes[name]; ok {
					out[name] = t
				}
			}
		}
	}
	return out, nil
}

func columnTypesFromTableInfo(ctx context.Context, db *sql.DB, table string) (map[string]string, error) {
	q := fmt.Sprintf(`PRAGMA table_info("%s")`, strings.ReplaceAll(table, `"`, `""`))
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, errs.Wrap(err, "pragma table_info", "table", table)
	}
	defer rows.Close()

	out := make(map[string]string)
	for rows.Next() {
		var cid int
		var name, declType string
		var notNull int
		var dfltValue any
		var pk int
		if err := rows.Scan(&cid, &name, &declType, &notNull, &dfltValue, &pk); err != nil {
			return nil, errs.Wrap(err, "scan table_info", "table", table)
		}
		out[name] = declType
	}
	return out, rows.Err()
}

func jsonColumnsFromDeclared(cols []string, declTypes map[string]string) map[string]bool {
	out := make(map[string]bool, len(cols))
	for _, name := range cols {
		if isDeclaredJSONColumnType(declTypes[name]) {
			out[name] = true
		}
	}
	return out
}

func normalizeScanCell(v any, parseJSON bool) any {
	if b, ok := v.([]byte); ok {
		v = string(b)
	}
	if !parseJSON {
		return v
	}
	return parseJSONCellValue(v)
}
