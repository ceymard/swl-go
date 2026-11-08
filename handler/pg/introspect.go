package pg

import (
	"context"

	"github.com/ceymard/swl-go/internal/errs"
	"github.com/jackc/pgx/v5"
)

// pgColumn is what the sink needs to know about one table column to validate
// incoming values before they reach Postgres.
type pgColumn struct {
	Name      string
	TypeOID   uint32 // base type OID (domains already resolved)
	TypeName  string // base type name, used in messages
	Typmod    int32  // atttypmod, or the domain's typtypmod when the column is a domain
	NotNull   bool   // column or domain NOT NULL
	HasDef    bool
	Generated bool     // GENERATED ... STORED, or identity ALWAYS: explicit values are refused
	EnumVals  []string // labels when the base type is an enum
}

// pgUnique is a non-partial, column-only unique index (or the primary key).
type pgUnique struct {
	Columns      []string
	NullsNotDist bool
	IsPrimary    bool
}

type pgTableMeta struct {
	Columns []pgColumn
	Uniques []pgUnique
}

func (m *pgTableMeta) column(name string) *pgColumn {
	for i := range m.Columns {
		if m.Columns[i].Name == name {
			return &m.Columns[i]
		}
	}
	return nil
}

// introspectTable reads column and unique-index metadata. It returns nil when
// the table does not exist (Postgres will report that itself at flush).
func introspectTable(ctx context.Context, tx pgx.Tx, fqTable string) (*pgTableMeta, error) {
	var oid *uint32
	if err := tx.QueryRow(ctx, `SELECT to_regclass($1)::oid`, fqTable).Scan(&oid); err != nil {
		return nil, errs.Wrap(err, "resolve table", "table", fqTable)
	}
	if oid == nil {
		return nil, nil
	}

	rows, err := tx.Query(ctx, `
		SELECT a.attname, a.atttypid, a.atttypmod, a.attnotnull, (a.atthasdef OR a.attidentity <> ''),
			(a.attidentity = 'a' OR a.attgenerated <> '')
		FROM pg_attribute a
		WHERE a.attrelid = $1 AND a.attnum > 0 AND NOT a.attisdropped
		ORDER BY a.attnum`, *oid)
	if err != nil {
		return nil, errs.Wrap(err, "introspect columns", "table", fqTable)
	}
	meta := &pgTableMeta{}
	for rows.Next() {
		var c pgColumn
		if err := rows.Scan(&c.Name, &c.TypeOID, &c.Typmod, &c.NotNull, &c.HasDef, &c.Generated); err != nil {
			rows.Close()
			return nil, errs.Wrap(err, "scan column", "table", fqTable)
		}
		meta.Columns = append(meta.Columns, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, errs.Wrap(err, "introspect columns", "table", fqTable)
	}

	for i := range meta.Columns {
		if err := resolveType(ctx, tx, &meta.Columns[i]); err != nil {
			return nil, err
		}
	}

	urows, err := tx.Query(ctx, `
		SELECT array_agg(a.attname ORDER BY k.ord), bool_or(i.indisprimary),
			bool_or(COALESCE((to_jsonb(i) ->> 'indnullsnotdistinct')::bool, false))
		FROM pg_index i
		CROSS JOIN LATERAL unnest(i.indkey::int2[]) WITH ORDINALITY AS k(attnum, ord)
		JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = k.attnum
		WHERE i.indrelid = $1 AND i.indisunique AND i.indisvalid
			AND i.indpred IS NULL AND i.indexprs IS NULL
			AND k.ord <= i.indnkeyatts
		GROUP BY i.indexrelid`, *oid)
	if err != nil {
		return nil, errs.Wrap(err, "introspect unique indexes", "table", fqTable)
	}
	defer urows.Close()
	for urows.Next() {
		var u pgUnique
		if err := urows.Scan(&u.Columns, &u.IsPrimary, &u.NullsNotDist); err != nil {
			return nil, errs.Wrap(err, "scan unique index", "table", fqTable)
		}
		meta.Uniques = append(meta.Uniques, u)
	}
	return meta, urows.Err()
}

// resolveType follows domains down to their base type (keeping domain NOT
// NULL and typmod) and loads enum labels. A domain's own CHECK constraints are
// not evaluated.
func resolveType(ctx context.Context, tx pgx.Tx, c *pgColumn) error {
	for depth := 0; depth < 16; depth++ {
		var (
			name    string
			typtype string
			base    uint32
			typmod  int32
			notNull bool
		)
		err := tx.QueryRow(ctx, `
			SELECT typname, typtype::text, typbasetype, typtypmod, typnotnull
			FROM pg_type WHERE oid = $1`, c.TypeOID).Scan(&name, &typtype, &base, &typmod, &notNull)
		if err != nil {
			return errs.Wrap(err, "introspect type", "column", c.Name)
		}
		c.TypeName = name
		switch typtype {
		case "d":
			if notNull {
				c.NotNull = true
			}
			if typmod != -1 {
				c.Typmod = typmod
			}
			c.TypeOID = base
		case "e":
			rows, err := tx.Query(ctx, `SELECT enumlabel FROM pg_enum WHERE enumtypid = $1`, c.TypeOID)
			if err != nil {
				return errs.Wrap(err, "introspect enum", "column", c.Name)
			}
			for rows.Next() {
				var l string
				if err := rows.Scan(&l); err != nil {
					rows.Close()
					return errs.Wrap(err, "scan enum label", "column", c.Name)
				}
				c.EnumVals = append(c.EnumVals, l)
			}
			rows.Close()
			return rows.Err()
		default:
			return nil
		}
	}
	return nil
}
