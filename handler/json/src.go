package json

import (
	"context"
	"os"
	"strconv"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/errs"
	"github.com/ceymard/swl-go/internal/handlers"
	"github.com/ceymard/swl-go/internal/stream"
	"github.com/aeolun/json5"
)

// Source reads JSON5 and emits one or more collections.
type Source struct{}

func (Source) Source(ctx context.Context, cfg handlers.Config, raw any) (coll.Stream, error) {
	opts := raw.(SrcOpts)
	if opts.File == "" {
		return nil, errs.New("json source requires a file path or inline JSON")
	}

	inline := fileIsInlineJSON(opts.File)
	var contents string
	if inline {
		contents = opts.File
	} else {
		enc := "utf-8"
		if opts.Encoding != nil {
			enc = *opts.Encoding
		}
		if enc != "utf-8" && enc != "" {
			return nil, errs.New("json source: only utf-8 encoding is supported")
		}
		b, err := os.ReadFile(opts.File)
		if err != nil {
			return nil, errs.Wrap(err, "read json file", "path", opts.File)
		}
		contents = string(b)
	}

	var parsed any
	if err := json5.Unmarshal([]byte(contents), &parsed); err != nil {
		return nil, errs.Wrap(err, "parse json5", "path", opts.File)
	}

	collections, err := normalizeParsed(parsed, opts.File, opts.Collection, inline)
	if err != nil {
		return nil, err
	}

	var cols []coll.Collection
	for name, rows := range collections {
		cols = append(cols, coll.Collection{
			Name: name,
			Rows: coll.SliceRows(rows),
		})
	}
	return stream.Of(cols...), nil
}

// normalizeParsed converts parsed JSON into named row slices (swl2 swl-json-src.ts).
func normalizeParsed(parsed any, source string, collection *string, inline bool) (map[string][]coll.Row, error) {
	defaultName := defaultCollectionName(source, inline)
	if collection != nil {
		defaultName = *collection
	}

	switch v := parsed.(type) {
	case []any:
		rows, err := objectsToRows(v)
		if err != nil {
			return nil, err
		}
		return map[string][]coll.Row{defaultName: rows}, nil

	case map[string]any:
		if inline {
			row, err := valueToRow(v)
			if err != nil {
				return nil, err
			}
			return map[string][]coll.Row{defaultName: {row}}, nil
		}
		out := make(map[string][]coll.Row, len(v))
		for name, val := range v {
			arr, ok := val.([]any)
			if !ok {
				return nil, errs.New("json object values must be arrays of objects: " + name)
			}
			rows, err := objectsToRows(arr)
			if err != nil {
				return nil, errs.Wrap(err, "collection "+name)
			}
			out[name] = rows
		}
		return out, nil

	default:
		return nil, errs.New("json root must be an object or array")
	}
}

func objectsToRows(items []any) ([]coll.Row, error) {
	rows := make([]coll.Row, 0, len(items))
	for i, item := range items {
		row, err := valueToRow(item)
		if err != nil {
			return nil, errs.Wrap(err, "row "+itoa(i))
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func valueToRow(v any) (coll.Row, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, errs.New("json array elements must be objects")
	}
	return coll.Row(m), nil
}

func itoa(i int) string {
	return strconv.Itoa(i)
}
