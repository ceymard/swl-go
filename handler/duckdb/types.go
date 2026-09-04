package duckdb

import (
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

// inferColumnType picks a DuckDB DDL type from a sample cell (swl2 duckdb-sink-common).
func inferColumnType(v any) string {
	switch v.(type) {
	case nil:
		return "VARCHAR"
	case bool:
		return "BOOLEAN"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return "BIGINT"
	case float32, float64:
		return "DOUBLE"
	case []byte:
		return "BLOB"
	case map[string]any, []any:
		return "JSON"
	default:
		return "VARCHAR"
	}
}

// normalizeCell converts nested cell values (a JSON blob column's own
// contents, not a top-level row) — these stay as generic map[string]any/
// []any, unrelated to any ColumnSet.
func normalizeCell(v any) any {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = normalizeCell(val)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = normalizeCell(val)
		}
		return out
	case []byte:
		return string(x)
	default:
		return v
	}
}

// rowFromJSONCell builds a positional row against cs from a to_json(...)
// scan result (a map, a JSON string, or raw bytes to be parsed as one).
func rowFromJSONCell(cs *coll.ColumnSet, v any) (coll.Row, error) {
	switch x := v.(type) {
	case map[string]any:
		normalized := make(map[string]any, len(x))
		for k, val := range x {
			normalized[k] = normalizeCell(val)
		}
		return coll.RowFromMap(cs, normalized), nil
	case string:
		var m map[string]any
		if err := jsonx.Unmarshal([]byte(x), &m); err != nil {
			return nil, err
		}
		return rowFromJSONCell(cs, m)
	case []byte:
		return rowFromJSONCell(cs, string(x))
	default:
		return coll.RowFromMap(cs, nil), nil
	}
}

func quoteIdent(name string) string {
	return `"` + name + `"`
}

func splitSchemaTable(name string) (schema, table string) {
	schema = "main"
	table = name
	if i := indexSchemaDot(name); i >= 0 {
		schema = name[:i]
		table = name[i+1:]
	}
	return schema, table
}

func indexSchemaDot(name string) int {
	for i := len(name) - 1; i >= 0; i-- {
		if name[i] == '.' {
			return i
		}
	}
	return -1
}
