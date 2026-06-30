package sqlite

import (
	"math"
	"sort"
	"time"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/jsonx"
)

// inferColumnType picks a SQLite DDL type from a sample cell value (swl2 sqlite-sink).
func inferColumnType(v any) string {
	switch x := v.(type) {
	case nil:
		return "TEXT"
	case bool:
		return "INTEGER"
	case int, int32, int64, uint, uint32, uint64:
		return "INTEGER"
	case float32:
		if math.Mod(float64(x), 1) == 0 {
			return "INTEGER"
		}
		return "REAL"
	case float64:
		if math.Mod(x, 1) == 0 {
			return "INTEGER"
		}
		return "REAL"
	case []byte:
		return "BLOB"
	case time.Time:
		return "TEXT"
	case map[string]any, []any:
		return "JSON"
	default:
		return "TEXT"
	}
}

// columnNames returns sorted keys from a row (stable DDL/INSERT order).
func columnNames(row coll.Row) []string {
	names := make([]string, 0, len(row))
	for k := range row {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}

// bindValue converts a row cell to a SQLite driver value (swl2 data() mapping).
func bindValue(v any) (any, error) {
	switch x := v.(type) {
	case nil:
		return nil, nil
	case time.Time:
		return x.UTC().Format(time.RFC3339Nano), nil
	case bool:
		if x {
			return int64(1), nil
		}
		return int64(0), nil
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

// maybeParseJSON unmarshals string cells that look like JSON objects/arrays.
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
	return out
}
