package json

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/errs"
	"github.com/ceymard/swl-go/internal/handlers"
	"github.com/ceymard/swl-go/internal/jsonx"
)

// Sink writes collections to JSON files (swl2 swl-json-sink.ts).
type Sink struct{}

func (Sink) Sink(ctx context.Context, cfg handlers.Config, in coll.Stream, raw any) error {
	opts := raw.(SinkOpts)
	path := opts.Path
	if path == "" {
		wd, err := os.Getwd()
		if err != nil {
			return errs.Wrap(err, "json sink cwd")
		}
		path = wd
	}

	stat, err := os.Stat(path)
	isDir := err == nil && stat.Mode().IsDir()
	usePercent := strings.Contains(path, "%")
	var singleFile bool

	var w io.WriteCloser
	if !isDir && !usePercent {
		singleFile = true
		f, err := os.Create(path)
		if err != nil {
			return errs.Wrap(err, "create json file", "path", path)
		}
		w = f
		defer w.Close()
		if opts.Object {
			if _, err := io.WriteString(w, "{\n"); err != nil {
				return err
			}
		}
	}

	firstCollection := true
	for c, err := range in {
		if err != nil {
			return err
		}
		colPath := path
		if isDir {
			colPath = filepath.Join(path, c.Name+".json")
		} else if usePercent {
			colPath = strings.Replace(path, "%", c.Name, 1)
		}

		var cw io.WriteCloser
		if singleFile {
			cw = &nopCloser{w}
		} else {
			f, err := os.Create(colPath)
			if err != nil {
				return errs.Wrap(err, "create json file", "path", colPath)
			}
			cw = f
		}

		if err := writeCollection(cw, c, opts.Object, !firstCollection && singleFile); err != nil {
			if !singleFile {
				_ = cw.Close()
			}
			return err
		}
		if !singleFile {
			if err := cw.Close(); err != nil {
				return errs.Wrap(err, "close json file", "path", colPath)
			}
		}
		firstCollection = false
	}

	if singleFile && opts.Object {
		if _, err := io.WriteString(w, "\n}\n"); err != nil {
			return err
		}
	}
	return nil
}

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }

func writeCollection(w io.Writer, c coll.Collection, objectMode, prefixComma bool) error {
	if prefixComma {
		if _, err := io.WriteString(w, ","); err != nil {
			return err
		}
	}
	if objectMode {
		if _, err := io.WriteString(w, `"`+c.Name+`": [`); err != nil {
			return err
		}
	} else {
		if _, err := io.WriteString(w, "["); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w, "\n"); err != nil {
		return err
	}

	// names tracks c.Columns' current names, refreshed only when the
	// (append-only, growing) ColumnSet has gained columns since last row —
	// JSON output has no fixed-width constraint, so a row can reference any
	// column discovered up to that point in the stream.
	var names []string
	first := true
	for batch, err := range c.Rows {
		if err != nil {
			return err
		}
		for _, row := range batch {
			if c.Columns != nil && len(names) != c.Columns.Len() {
				cs := c.Columns.Columns()
				names = make([]string, len(cs))
				for i, col := range cs {
					names[i] = col.ColumnName
				}
			}
			if !first {
				if _, err := io.WriteString(w, ",\n"); err != nil {
					return err
				}
			}
			first = false
			b, err := marshalRow(names, row)
			if err != nil {
				return errs.Wrap(err, "marshal json row", "collection", c.Name)
			}
			if _, err := w.Write(b); err != nil {
				return err
			}
		}
	}
	_, err := io.WriteString(w, "\n]\n")
	return err
}

// marshalRow encodes row as a JSON object directly from (names, cells) —
// no intermediate map[string]any allocation. names[i] pairs with row[i];
// a row shorter than names (built before later columns were discovered)
// simply omits the trailing, not-yet-known keys.
func marshalRow(names []string, row coll.Row) ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	n := len(row)
	if len(names) < n {
		n = len(names)
	}
	for i := 0; i < n; i++ {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := jsonx.Marshal(names[i])
		if err != nil {
			return nil, err
		}
		buf.Write(key)
		buf.WriteByte(':')
		val, err := jsonx.Marshal(row.Cell(i))
		if err != nil {
			return nil, err
		}
		buf.Write(val)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}
