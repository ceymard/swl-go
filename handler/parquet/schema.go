package parquet

import (
	"os"
	"sort"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/parquet-go/parquet-go"
)

func buildSchema(rows []map[string]any) *parquet.Schema {
	group := parquet.Group{}
	cols := columnNamesFromRows(rows)
	for _, col := range cols {
		group[col] = inferColumnType(rows, col)
	}
	return parquet.NewSchema("rows", group)
}

func columnNamesFromRows(rows []map[string]any) []string {
	seen := make(map[string]struct{}, len(rows))
	var cols []string
	for _, row := range rows {
		for k := range row {
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			cols = append(cols, k)
		}
	}
	sort.Strings(cols)
	return cols
}

func inferColumnType(rows []map[string]any, col string) parquet.Node {
	for _, row := range rows {
		if v, ok := row[col]; ok && v != nil {
			return inferNode(v)
		}
	}
	return parquet.Optional(parquet.String())
}

func inferNode(v any) parquet.Node {
	switch v.(type) {
	case bool:
		return parquet.Leaf(parquet.BooleanType)
	case int, int8, int16, int32, int64:
		return parquet.Int(64)
	case uint, uint8, uint16, uint32, uint64:
		return parquet.Int(64)
	case float32:
		return parquet.Leaf(parquet.FloatType)
	case float64:
		return parquet.Leaf(parquet.DoubleType)
	case []byte:
		return parquet.Leaf(parquet.ByteArrayType)
	case string:
		return parquet.String()
	default:
		return parquet.String()
	}
}

func writeParquetFile(path string, rows []map[string]any) error {
	if len(rows) == 0 {
		rows = []map[string]any{}
	}
	schema := buildSchema(rows)
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := parquet.NewGenericWriter[any](f, schema)
	payload := make([]any, len(rows))
	for i, row := range rows {
		payload[i] = row
	}
	if _, err := writer.Write(payload); err != nil {
		return err
	}
	return writer.Close()
}

func anyToRow(v any) coll.Row {
	if m, ok := v.(map[string]any); ok {
		return coll.Row(m)
	}
	if m, ok := v.(map[string]interface{}); ok {
		out := make(coll.Row, len(m))
		for k, val := range m {
			out[k] = val
		}
		return out
	}
	return coll.Row{}
}
