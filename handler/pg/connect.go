package pg

import (
	"context"
	"strings"

	"github.com/ceymard/swl-go/internal/errs"
	"github.com/ceymard/swl-go/internal/ssh"
	"github.com/jackc/pgx/v5/pgxpool"
)

const pgDefaultPort = 5432

// connect opens a pgx pool, optionally via an SSH tunnel (swl2 uri_maybe_open_tunnel).
func connect(ctx context.Context, uri string) (*pgxpool.Pool, *ssh.OpenResult, error) {
	open, err := ssh.MaybeOpenTunnel(uri, pgDefaultPort)
	if err != nil {
		return nil, nil, err
	}
	connURI := normalizePostgresURI(open.URI)
	cfg, err := pgxpool.ParseConfig(connURI)
	if err != nil {
		_ = open.Close()
		return nil, nil, errs.Wrap(err, "parse postgres uri")
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		_ = open.Close()
		return nil, nil, errs.Wrap(err, "connect postgres")
	}
	return pool, open, nil
}

func normalizePostgresURI(uri string) string {
	if strings.HasPrefix(uri, "postgres://") || strings.HasPrefix(uri, "postgresql://") {
		return uri
	}
	return "postgres://" + uri
}

func qualifyTable(name, defaultSchema string) (schema, table string) {
	if strings.Contains(name, ".") {
		parts := strings.SplitN(name, ".", 2)
		return parts[0], parts[1]
	}
	return defaultSchema, name
}

func querySQL(spec TableSpec) string {
	if spec.Query != "" {
		return spec.Query
	}
	schema, table := qualifyTable(spec.Name, "public")
	return `SELECT * FROM "` + schema + `"."` + table + `" TBL`
}

func listSchemaTables(ctx context.Context, pool *pgxpool.Pool, schema string) ([]TableSpec, error) {
	rows, err := pool.Query(ctx, `
		SELECT table_schema || '.' || table_name
		FROM information_schema.tables
		WHERE table_schema = $1 AND table_type = 'BASE TABLE'
		ORDER BY 1`, schema)
	if err != nil {
		return nil, errs.Wrap(err, "list postgres tables", "schema", schema)
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

func expandWildcardSources(ctx context.Context, pool *pgxpool.Pool, sources []TableSpec) ([]TableSpec, error) {
	var out []TableSpec
	for _, s := range sources {
		if strings.HasSuffix(s.Name, ".*") {
			schema := strings.TrimSuffix(s.Name, ".*")
			tables, err := listSchemaTables(ctx, pool, schema)
			if err != nil {
				return nil, err
			}
			out = append(out, tables...)
			continue
		}
		out = append(out, s)
	}
	return out, nil
}
