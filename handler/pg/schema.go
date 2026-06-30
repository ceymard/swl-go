package pg

import (
	"context"

	"github.com/ceymard/swl-go/internal/errs"
	"github.com/jackc/pgx/v5/pgxpool"
)

// listSchemaTables returns base tables in a schema, ordered so FK dependencies
// appear before dependents (swl2 get_all_tables_from_schema).
func listSchemaTables(ctx context.Context, pool *pgxpool.Pool, schema string) ([]TableSpec, error) {
	rows, err := pool.Query(ctx, `
		WITH cons AS (
			SELECT
				tc.table_schema,
				tc.constraint_name,
				tc.table_name,
				kcu.column_name,
				ccu.table_schema AS foreign_table_schema,
				ccu.table_name AS foreign_table_name,
				ccu.column_name AS foreign_column_name
			FROM information_schema.table_constraints AS tc
			JOIN information_schema.key_column_usage AS kcu
				ON tc.constraint_name = kcu.constraint_name
				AND tc.table_schema = kcu.table_schema
			JOIN information_schema.constraint_column_usage AS ccu
				ON ccu.constraint_name = tc.constraint_name
				AND ccu.table_schema = tc.table_schema
			WHERE tc.constraint_type = 'FOREIGN KEY'
		),
		tbls AS (
			SELECT
				tbl.table_schema AS schema,
				tbl.table_schema || '.' || tbl.table_name AS tbl,
				cons.foreign_table_schema || '.' || cons.foreign_table_name AS dep
			FROM information_schema.tables tbl
			LEFT JOIN cons
				ON cons.table_name = tbl.table_name
				AND cons.table_schema = tbl.table_schema
			WHERE tbl.table_schema = $1
				AND tbl.table_type = 'BASE TABLE'
		)
		SELECT
			t.tbl,
			COALESCE(array_agg(t.dep) FILTER (WHERE t.dep IS NOT NULL), '{}'::text[]) AS deps
		FROM tbls t
		GROUP BY t.tbl`, schema)
	if err != nil {
		return nil, errs.Wrap(err, "list postgres tables with dependencies", "schema", schema)
	}
	defer rows.Close()

	deps := make(map[string][]string)
	for rows.Next() {
		var tbl string
		var depArr []string
		if err := rows.Scan(&tbl, &depArr); err != nil {
			return nil, errs.Wrap(err, "scan table dependency row")
		}
		deps[tbl] = depArr
	}
	if err := rows.Err(); err != nil {
		return nil, errs.Wrap(err, "list postgres table dependencies")
	}

	names := orderTablesByFKDeps(deps)
	specs := make([]TableSpec, len(names))
	for i, name := range names {
		specs[i] = TableSpec{Name: name}
	}
	return specs, nil
}

// orderTablesByFKDeps topologically orders tables: referenced tables first.
func orderTablesByFKDeps(deps map[string][]string) []string {
	seen := make(map[string]bool, len(deps))
	var order []string

	var visit func(tbl string)
	visit = func(tbl string) {
		if seen[tbl] {
			return
		}
		for _, dep := range deps[tbl] {
			if dep == tbl {
				continue
			}
			if _, ok := deps[dep]; ok && !seen[dep] {
				visit(dep)
			}
		}
		seen[tbl] = true
		order = append(order, tbl)
	}

	for tbl := range deps {
		visit(tbl)
	}
	return order
}
