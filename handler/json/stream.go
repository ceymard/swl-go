package json

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"sort"

	"github.com/aeolun/json5"
	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/errs"
	"github.com/ceymard/swl-go/internal/jsonx"
)

// streamFromBytes emits collections without materializing all rows upfront.
func streamFromBytes(ctx context.Context, data []byte, source string, collection *string, inline bool) (coll.Stream, error) {
	if len(data) == 0 {
		return nil, errs.New("empty json input")
	}
	defaultName := defaultCollectionName(source, inline)
	if collection != nil {
		defaultName = *collection
	}

	switch data[0] {
	case '[':
		return streamSingleCollection(ctx, defaultName, data), nil
	case '{':
		if inline {
			var row coll.Row
			if err := parseJSON(data, &row); err != nil {
				return nil, err
			}
			return singleCollectionStream(ctx, defaultName, coll.SliceRowBatches([]coll.Row{row})), nil
		}
		return streamObjectCollections(ctx, data)
	default:
		return nil, errs.New("json root must be an object or array")
	}
}

func parseJSON(data []byte, v any) error {
	if err := jsonx.Unmarshal(data, v); err != nil {
		if err5 := json5.Unmarshal(data, v); err5 != nil {
			return errs.Wrap(err, "parse json")
		}
	}
	return nil
}

func streamObjectCollections(ctx context.Context, data []byte) (coll.Stream, error) {
	var root map[string]json.RawMessage
	if err := parseJSON(data, &root); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(root))
	for name := range root {
		names = append(names, name)
	}
	sort.Strings(names)

	return func(yield func(coll.Collection, error) bool) {
		for _, name := range names {
			if err := ctx.Err(); err != nil {
				yield(coll.Collection{}, err)
				return
			}
			raw := root[name]
			if len(raw) == 0 {
				continue
			}
			rows := streamJSONArray(ctx, raw)
			c := coll.Collection{Name: name, Rows: rows}
			if !yield(c, nil) {
				return
			}
		}
	}, nil
}

func streamSingleCollection(ctx context.Context, name string, data []byte) coll.Stream {
	return func(yield func(coll.Collection, error) bool) {
		c := coll.Collection{Name: name, Rows: streamJSONArray(ctx, data)}
		yield(c, nil)
	}
}

func singleCollectionStream(ctx context.Context, name string, rows coll.RowBatches) coll.Stream {
	return func(yield func(coll.Collection, error) bool) {
		yield(coll.Collection{Name: name, Rows: rows}, nil)
	}
}

func streamJSONArray(ctx context.Context, data []byte) coll.RowBatches {
	data = bytes.TrimSpace(data)
	if len(data) > 0 && data[0] == '[' {
		return streamJSONArrayElements(ctx, data)
	}
	return streamJSONValues(ctx, bytes.NewReader(data))
}

// streamJSONArrayElements walks a standard JSON array by token, decoding
// elements in batches without materializing the whole array.
func streamJSONArrayElements(ctx context.Context, data []byte) coll.RowBatches {
	return func(yield func([]coll.Row, error) bool) {
		dec := jsonx.NewStreamDecoder(bytes.NewReader(data))
		if _, err := dec.Token(); err != nil { // consume opening '['
			yield(nil, errs.Wrap(err, "decode json array"))
			return
		}
		batch := make([]coll.Row, 0, coll.DefaultBatchSize)
		for dec.More() {
			if err := ctx.Err(); err != nil {
				yield(nil, err)
				return
			}
			var row coll.Row
			if err := dec.Decode(&row); err != nil {
				yield(nil, errs.Wrap(err, "decode json row"))
				return
			}
			batch = append(batch, row)
			if len(batch) == coll.DefaultBatchSize {
				if !yield(batch, nil) {
					return
				}
				batch = make([]coll.Row, 0, coll.DefaultBatchSize)
			}
		}
		if len(batch) > 0 {
			yield(batch, nil)
		}
	}
}

// streamJSONValues decodes concatenated top-level JSON values (NDJSON-style)
// in batches.
func streamJSONValues(ctx context.Context, r io.Reader) coll.RowBatches {
	return func(yield func([]coll.Row, error) bool) {
		dec := jsonx.NewStreamDecoder(r)
		batch := make([]coll.Row, 0, coll.DefaultBatchSize)
		for dec.More() {
			if err := ctx.Err(); err != nil {
				yield(nil, err)
				return
			}
			var row coll.Row
			if err := dec.Decode(&row); err != nil {
				yield(nil, errs.Wrap(err, "decode json row"))
				return
			}
			batch = append(batch, row)
			if len(batch) == coll.DefaultBatchSize {
				if !yield(batch, nil) {
					return
				}
				batch = make([]coll.Row, 0, coll.DefaultBatchSize)
			}
		}
		if len(batch) > 0 {
			yield(batch, nil)
		}
	}
}
