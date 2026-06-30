package duckdb

import (
	"sort"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/jsonx"
)

func columnNames(row coll.Row) []string {
	names := make([]string, 0, len(row))
	for k := range row {
		names = append(names, k)
	}
	sort.Strings(names)
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

func normalizeCell(v any) any {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case map[string]any:
		row := make(coll.Row, len(x))
		for k, val := range x {
			row[k] = normalizeCell(val)
		}
		return row
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

func rowFromJSONCell(v any) (coll.Row, error) {
	switch x := v.(type) {
	case map[string]any:
		row := make(coll.Row, len(x))
		for k, val := range x {
			row[k] = normalizeCell(val)
		}
		return row, nil
	case string:
		var m map[string]any
		if err := jsonx.Unmarshal([]byte(x), &m); err != nil {
			return nil, err
		}
		return rowFromJSONCell(m)
	case []byte:
		return rowFromJSONCell(string(x))
	default:
		return coll.Row{}, nil
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
