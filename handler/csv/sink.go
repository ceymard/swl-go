package csv

import (
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/errs"
	"github.com/ceymard/swl-go/internal/handlers"
)

// Sink writes collections to CSV files (swl2 swl-csv-sink.ts).
type Sink struct{}

func (Sink) Sink(ctx context.Context, cfg handlers.Config, in coll.Stream, raw any) error {
	opts := raw.(SinkOpts)
	if opts.Path == "" {
		return errs.New("csv sink requires an output path")
	}
	if opts.Charset != "" && opts.Charset != "utf-8" {
		return errs.New("csv sink: only utf-8 charset is supported")
	}
	return handlers.ConsumeHooks(cfg, &sinkHooks{cfg: cfg, opts: opts}, in)
}

type sinkHooks struct {
	cfg  handlers.Config
	opts SinkOpts
}

func (h *sinkHooks) Init(ctx context.Context) error { return nil }
func (h *sinkHooks) Rollback(ctx context.Context)   {}
func (h *sinkHooks) Finish(ctx context.Context) error {
	if h.cfg.Messages != nil {
		h.cfg.Log(2, "Finished csv sink")
	}
	return nil
}

func (h *sinkHooks) Open(ctx context.Context, col coll.Collection, firstRow coll.Row) (handlers.RowWriter, error) {
	path, err := sinkPath(h.opts.Path, col.Name)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && filepath.Dir(path) != "." {
		return nil, errs.Wrap(err, "create csv directory", "path", path)
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, errs.Wrap(err, "create csv file", "path", path)
	}
	if h.cfg.Messages != nil {
		h.cfg.Log(2, "opening", path, "for writing")
	}

	w := csv.NewWriter(f)
	w.Comma = h.opts.Delimiter

	cols := columnNames(col)
	if !h.opts.NoHeaders {
		if err := w.Write(cols); err != nil {
			_ = f.Close()
			return nil, errs.Wrap(err, "write csv header", "path", path)
		}
	}

	rw := &rowWriter{w: w, f: f, cols: cols, path: path}
	if err := rw.writeRow(firstRow); err != nil {
		_ = f.Close()
		return nil, err
	}
	return rw, nil
}

type rowWriter struct {
	w    *csv.Writer
	f    *os.File
	cols []string
	path string
}

func (rw *rowWriter) Write(row coll.Row) error {
	return rw.writeRow(row)
}

func (rw *rowWriter) Close() error {
	rw.w.Flush()
	if err := rw.w.Error(); err != nil {
		return errs.Wrap(err, "flush csv", "path", rw.path)
	}
	return rw.f.Close()
}

func (rw *rowWriter) writeRow(row coll.Row) error {
	record := make([]string, len(rw.cols))
	for i := range rw.cols {
		record[i] = cellString(row.Cell(i))
	}
	if err := rw.w.Write(record); err != nil {
		return errs.Wrap(err, "write csv row", "path", rw.path)
	}
	return nil
}

// columnNames snapshots col's columns at Open time, in natural discovery
// order (no sort — see plan's "Sink output order"). Columns appearing in
// rows after this snapshot are silently dropped, matching prior behavior.
func columnNames(col coll.Collection) []string {
	if col.Columns == nil {
		return nil
	}
	cs := col.Columns.Columns()
	cols := make([]string, len(cs))
	for i, c := range cs {
		cols[i] = c.ColumnName
	}
	return cols
}

func cellString(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case fmt.Stringer:
		return x.String()
	default:
		return fmt.Sprint(x)
	}
}

// sinkPath resolves the output file for a collection (swl2 path rules).
func sinkPath(base, colName string) (string, error) {
	if strings.Contains(base, ".csv") {
		if strings.Contains(base, "%") {
			return strings.ReplaceAll(base, "%", colName), nil
		}
		return base, nil
	}
	if strings.Contains(base, "%") {
		return strings.ReplaceAll(base, "%", colName), nil
	}
	info, err := os.Stat(base)
	if err == nil && info.IsDir() {
		return filepath.Join(base, colName+".csv"), nil
	}
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	// path does not exist — treat as directory if no extension
	if filepath.Ext(base) == "" {
		return filepath.Join(base, colName+".csv"), nil
	}
	return base, nil
}
