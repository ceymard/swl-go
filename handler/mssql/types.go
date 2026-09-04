package mssql

import (
	"math"
	"strings"
	"time"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/jsonx"
)

// columnNames snapshots col's columns at Open time, in natural discovery
// order (no sort — see plan's "Sink output order"). Columns appearing in
// rows after this snapshot are silently dropped, matching prior behavior.
func columnNames(col coll.Collection) []string {
	if col.Columns == nil {
		return nil
	}
	cs := col.Columns.Columns()
	names := make([]string, len(cs))
	for i, c := range cs {
		names[i] = c.ColumnName
	}
	return names
}

func splitTable(name string) (schema, table string) {
	parts := strings.SplitN(name, ".", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "dbo", parts[0]
}

func quoteIdent(name string) string {
	return "[" + strings.ReplaceAll(name, "]", "]]") + "]"
}

func quoteTable(name string) string {
	schema, table := splitTable(name)
	return quoteIdent(schema) + "." + quoteIdent(table)
}

func quoteColumn(name string) string {
	return quoteIdent(name)
}

func inferColumnType(v any) string {
	switch x := v.(type) {
	case nil:
		return "NVARCHAR(MAX)"
	case bool:
		return "BIT"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return "BIGINT"
	case float32:
		if math.Mod(float64(x), 1) == 0 {
			return "BIGINT"
		}
		return "FLOAT"
	case float64:
		if math.Mod(x, 1) == 0 {
			return "BIGINT"
		}
		return "FLOAT"
	case []byte:
		return "VARBINARY(MAX)"
	case time.Time:
		return "DATETIME2(7)"
	case map[string]any, []any:
		return "NVARCHAR(MAX)"
	default:
		return "NVARCHAR(MAX)"
	}
}

func normalizeCell(v any) any {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case []byte:
		return maybeParseJSON(string(x))
	default:
		return maybeParseJSON(v)
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
	return normalizeCell(out)
}

func bindValue(v any) (any, error) {
	switch x := v.(type) {
	case nil:
		return nil, nil
	case time.Time:
		return x.UTC(), nil
	case map[string]any, []any:
		b, err := jsonx.Marshal(x)
		if err != nil {
			return nil, err
		}
		return string(b), nil
	default:
		return v, nil
	}
}
