package yaml

import (
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

// Sink writes collections to YAML files (swl2 swl-yaml-sink.ts).
type Sink struct{}

func (Sink) Sink(ctx context.Context, cfg handlers.Config, in coll.Stream, raw any) error {
	opts := raw.(SinkOpts)
	path := opts.Path
	if path == "" {
		wd, err := os.Getwd()
		if err != nil {
			return errs.Wrap(err, "yaml sink cwd")
		}
		path = wd
	}

	stat, err := os.Stat(path)
	isDir := err == nil && stat.IsDir()
	usePercent := strings.Contains(path, "%")
	var singleFile bool

	var w io.WriteCloser
	if !isDir && !usePercent {
		singleFile = true
		f, err := os.Create(path)
		if err != nil {
			return errs.Wrap(err, "create yaml file", "path", path)
		}
		w = f
		defer w.Close()
	}

	for c, err := range in {
		if err != nil {
			return err
		}
		colPath := path
		if isDir {
			colPath = filepath.Join(path, c.Name+".yml")
		} else if usePercent {
			colPath = strings.Replace(path, "%", c.Name, 1)
		}

		var cw io.WriteCloser
		if singleFile {
			if _, err := io.WriteString(w, c.Name+":\n"); err != nil {
				return err
			}
			cw = &nopCloser{w}
		} else {
			f, err := os.Create(colPath)
			if err != nil {
				return errs.Wrap(err, "create yaml file", "path", colPath)
			}
			cw = f
		}

		if err := writeCollection(cw, c); err != nil {
			if !singleFile {
				_ = cw.Close()
			}
			return err
		}
		if !singleFile {
			if err := cw.Close(); err != nil {
				return errs.Wrap(err, "close yaml file", "path", colPath)
			}
		}
	}

	if cfg.Messages != nil {
		cfg.Log(2, "Finished yaml sink")
	}
	return nil
}

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }

func writeCollection(w io.Writer, c coll.Collection) error {
	for batch, err := range c.Rows {
		if err != nil {
			return err
		}
		for _, row := range batch {
			b, err := jsonx.Marshal(row)
			if err != nil {
				return errs.Wrap(err, "marshal yaml row", "collection", c.Name)
			}
			if _, err := io.WriteString(w, "- "); err != nil {
				return err
			}
			if _, err := w.Write(b); err != nil {
				return err
			}
			if _, err := io.WriteString(w, "\n"); err != nil {
				return err
			}
		}
	}
	return nil
}

var _ handlers.Sink = Sink{}
