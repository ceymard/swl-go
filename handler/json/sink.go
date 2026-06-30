package json

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/errs"
	"github.com/ceymard/swl-go/internal/handlers"
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

	first := true
	for row, err := range c.Rows {
		if err != nil {
			return err
		}
		if !first {
			if _, err := io.WriteString(w, ",\n"); err != nil {
				return err
			}
		}
		first = false
		b, err := json.Marshal(row)
		if err != nil {
			return errs.Wrap(err, "marshal json row", "collection", c.Name)
		}
		if _, err := w.Write(b); err != nil {
			return err
		}
	}
	_, err := io.WriteString(w, "\n]\n")
	return err
}
