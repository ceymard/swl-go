package yaml

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/errs"
	"github.com/ceymard/swl-go/internal/handlers"
)

// Source reads YAML files and emits collections (swl2 swl-yaml-src.ts).
type Source struct{}

func (Source) Source(ctx context.Context, cfg handlers.Config, raw any) (coll.Stream, error) {
	opts := raw.(SrcOpts)
	if opts.File == "" {
		return nil, errs.New("yaml source requires a file path")
	}
	if opts.Encoding != nil && *opts.Encoding != "" && *opts.Encoding != "utf-8" {
		return nil, errs.New("yaml source: only utf-8 encoding is supported")
	}

	data, err := os.ReadFile(opts.File)
	if err != nil {
		return nil, errs.Wrap(err, "read yaml file", "path", opts.File)
	}

	defaultCollection := ""
	if opts.Collection != nil {
		defaultCollection = *opts.Collection
	}
	if defaultCollection == "" {
		base := filepath.Base(opts.File)
		defaultCollection = strings.TrimSuffix(base, filepath.Ext(base))
	}

	rt := newEvalRuntime()
	doc, err := parseDocument(data, defaultCollection, rt)
	if err != nil {
		return nil, err
	}

	return streamDocument(doc), nil
}

func streamDocument(doc *parsedDoc) coll.Stream {
	return func(yield func(coll.Collection, error) bool) {
		acc := make(map[string][]any)
		for _, name := range doc.keys {
			items := doc.data[name]
			if name == "__refs__" {
				acc["__refs__"] = items
				continue
			}

			rows, err := expandCollectionItems(items, acc)
			if err != nil {
				yield(coll.Collection{}, err)
				return
			}
			acc[name] = rows

			c := coll.Collection{
				Name: name,
				Rows: coll.SliceRowBatches(rowsToEmit(rows)),
			}
			if !yield(c, nil) {
				return
			}
		}
	}
}

func expandCollectionItems(items []any, acc map[string][]any) ([]any, error) {
	out := make([]any, 0, len(items))
	for _, item := range items {
		switch x := item.(type) {
		case *jsGenerator:
			before := len(out)
			err := x.run(acc, func(row coll.Row) error {
				out = append(out, row)
				return nil
			})
			if err != nil {
				return nil, err
			}
			if len(out) == before {
				continue
			}
		default:
			out = append(out, item)
		}
	}
	return out, nil
}

func rowsToEmit(stored []any) []coll.Row {
	rows := make([]coll.Row, 0, len(stored))
	for _, item := range stored {
		if row, ok := itemToRow(item); ok {
			rows = append(rows, stripMeta(row))
			continue
		}
		rows = append(rows, coll.Row{"value": item})
	}
	return rows
}

var _ handlers.Source = Source{}
