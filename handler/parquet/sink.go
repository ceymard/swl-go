package parquet

import (
	"context"
	"encoding/json"
	"iter"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/errs"
	"github.com/ceymard/swl-go/internal/handlers"
)

// Sink writes collections to Parquet files (.parquet / .pqt).
type Sink struct{}

func (Sink) Sink(ctx context.Context, cfg handlers.Config, in coll.Stream, raw any) error {
	opts := raw.(SinkOpts)
	if opts.Path == "" {
		return errs.New("parquet sink requires an output path")
	}

	singleFile := isSingleParquetPath(opts.Path)
	var wrote bool

	for c, err := range in {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if singleFile && wrote {
			return errs.New("parquet sink: multiple collections cannot write to a single file path")
		}

		rows, err := collectRows(c.Rows)
		if err != nil {
			return err
		}
		outPath, err := sinkPath(opts.Path, c.Name)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil && filepath.Dir(outPath) != "." {
			return errs.Wrap(err, "create parquet directory", "path", outPath)
		}
		if cfg.Messages != nil {
			cfg.Messages.Log(2, "writing", outPath)
		}
		if err := writeParquetFile(outPath, rows); err != nil {
			return errs.Wrap(err, "write parquet", "path", outPath)
		}
		wrote = true
	}
	return nil
}

func collectRows(rows iter.Seq2[coll.Row, error]) ([]map[string]any, error) {
	var out []map[string]any
	for row, err := range rows {
		if err != nil {
			return nil, err
		}
		out = append(out, normalizeRow(row))
	}
	return out, nil
}

func normalizeRow(row coll.Row) map[string]any {
	out := make(map[string]any, len(row))
	for k, v := range row {
		out[k] = sinkValue(v)
	}
	return out
}

func sinkValue(v any) any {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case string, bool,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return v
	case time.Time:
		return x.Format(time.RFC3339)
	case json.Number:
		if i, err := x.Int64(); err == nil {
			return i
		}
		if f, err := x.Float64(); err == nil {
			return f
		}
		return x.String()
	default:
		return v
	}
}

func isSingleParquetPath(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return (ext == ".parquet" || ext == ".pqt") && !strings.Contains(path, "%")
}

func sinkPath(base, colName string) (string, error) {
	ext := strings.ToLower(filepath.Ext(base))
	if ext == ".parquet" || ext == ".pqt" {
		if strings.Contains(base, "%") {
			return strings.ReplaceAll(base, "%", colName), nil
		}
		return base, nil
	}
	if strings.Contains(base, "%") {
		replaced := strings.ReplaceAll(base, "%", colName)
		if filepath.Ext(replaced) == "" {
			return replaced + ".parquet", nil
		}
		return replaced, nil
	}
	info, err := os.Stat(base)
	if err == nil && info.IsDir() {
		return filepath.Join(base, colName+".parquet"), nil
	}
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if ext == "" {
		return filepath.Join(base, colName+".parquet"), nil
	}
	return base, nil
}
