package mysql

import (
	"math"
	"sort"
	"strings"
	"time"

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

func inferColumnType(v any) string {
	switch x := v.(type) {
	case nil:
		return "TEXT"
	case bool:
		return "BOOLEAN"
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		return "BIGINT"
	case float32:
		if math.Mod(float64(x), 1) == 0 {
			return "BIGINT"
		}
		return "DOUBLE"
	case float64:
		if math.Mod(x, 1) == 0 {
			return "BIGINT"
		}
		return "DOUBLE"
	case []byte:
		return "BLOB"
	case time.Time:
		return "DATETIME(6)"
	case map[string]any, []any:
		return "JSON"
	default:
		return "TEXT"
	}
}

func normalizeCell(v any) any {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case []byte:
		return maybeParseJSON(string(x))
	case map[string]any:
		out := make(coll.Row, len(x))
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

func quoteColumn(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}
