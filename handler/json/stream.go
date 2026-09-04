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
			var m map[string]any
			if err := parseJSON(data, &m); err != nil {
				return nil, err
			}
			cs := coll.NewColumnSet()
			row := coll.RowFromMap(cs, m)
			return singleCollectionStream(ctx, defaultName, cs, coll.SliceRowBatches([]coll.Row{row})), nil
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
			rows, cs := streamJSONArray(ctx, raw)
			c := coll.Collection{Name: name, Columns: cs, Rows: rows}
			if !yield(c, nil) {
				return
			}
		}
	}, nil
}

func streamSingleCollection(ctx context.Context, name string, data []byte) coll.Stream {
	return func(yield func(coll.Collection, error) bool) {
		rows, cs := streamJSONArray(ctx, data)
		c := coll.Collection{Name: name, Columns: cs, Rows: rows}
		yield(c, nil)
	}
}

func singleCollectionStream(ctx context.Context, name string, cs *coll.ColumnSet, rows coll.RowBatches) coll.Stream {
	return func(yield func(coll.Collection, error) bool) {
		yield(coll.Collection{Name: name, Columns: cs, Rows: rows}, nil)
	}
}

// streamJSONArray returns a RowBatches iterator plus the ColumnSet it grows
// as rows are decoded (empty until iteration starts).
func streamJSONArray(ctx context.Context, data []byte) (coll.RowBatches, *coll.ColumnSet) {
	data = bytes.TrimSpace(data)
	cs := coll.NewColumnSet()
	if len(data) > 0 && data[0] == '[' {
		return streamJSONArrayElements(ctx, cs, data), cs
	}
	return streamJSONValues(ctx, cs, bytes.NewReader(data)), cs
}

// streamJSONArrayElements walks a standard JSON array by token, decoding
// elements in batches without materializing the whole array. Each element's
// own top-level keys are walked in source order (see decodeRowObject) so cs
// assigns column indexes deterministically, matching the file's field order.
func streamJSONArrayElements(ctx context.Context, cs *coll.ColumnSet, data []byte) coll.RowBatches {
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
			row, err := decodeRowObject(dec, cs)
			if err != nil {
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
// in batches, in source key order per row (see decodeRowObject).
func streamJSONValues(ctx context.Context, cs *coll.ColumnSet, r io.Reader) coll.RowBatches {
	return func(yield func([]coll.Row, error) bool) {
		dec := jsonx.NewStreamDecoder(r)
		batch := make([]coll.Row, 0, coll.DefaultBatchSize)
		for dec.More() {
			if err := ctx.Err(); err != nil {
				yield(nil, err)
				return
			}
			row, err := decodeRowObject(dec, cs)
			if err != nil {
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

// rowDecoder is the subset of *encoding/json.Decoder (or sonic's aliased
// equivalent, guaranteed identical post the go1.27 bump) decodeRowObject
// needs to walk a row object's top-level keys in source order.
type rowDecoder interface {
	Token() (json.Token, error)
	More() bool
	Decode(v any) error
}

// decodeRowObject walks the row object directly off dec — a single pass, no
// raw-bytes capture and no sub-decoder — reading the row's own top-level
// keys in source order so cs.Index assigns column indexes deterministically.
// Nested values still decode generically into map[string]any/[]any.
func decodeRowObject(dec rowDecoder, cs *coll.ColumnSet) (coll.Row, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := tok.(json.Delim)
	if !ok || delim != '{' {
		return nil, errs.New("json array element must be an object")
	}

	row := make(coll.Row, cs.Len())
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, _ := keyTok.(string)
		var cell any
		if err := dec.Decode(&cell); err != nil {
			return nil, err
		}
		idx := cs.Index(key)
		if idx >= len(row) {
			grown := make(coll.Row, idx+1)
			copy(grown, row)
			row = grown
		}
		row[idx] = cell
	}
	if _, err := dec.Token(); err != nil { // consume closing '}'
		return nil, err
	}
	return row, nil
}
