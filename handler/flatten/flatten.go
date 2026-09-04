// Package flatten implements the flatten transform (swl2 swl-flatten.ts).
//
// Nested maps and arrays become dot/bracket flat keys, e.g. {a:{b:1}} → {"a.b":1}.
// Unflatten (used by unflatten handler) inverts this encoding.
package flatten

import (
	"context"
	"fmt"
	"strings"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/handlers"
	"github.com/ceymard/swl-go/internal/stream"
)

type Transform struct{}

type Options struct{} // no flags

func (Transform) Transform(ctx context.Context, cfg handlers.Config, in coll.Stream, _ any) (coll.Stream, error) {
	return stream.MapRows(in, func(c coll.Collection) (*coll.ColumnSet, func(coll.Row) (coll.Row, error)) {
		// flatten invents new dotted-path keys per row — sharing the input
		// ColumnSet would leave dead all-nil columns for the original
		// nested keys, so it always allocates a fresh output ColumnSet.
		inCols := c.Columns
		outCols := coll.NewColumnSet()
		cols := ColumnCache(inCols)
		return outCols, func(row coll.Row) (coll.Row, error) {
			return flattenRow(cols(), row, outCols), nil
		}
	}), nil
}

// ColumnCache returns a function yielding cs.Columns(), re-snapshotting only
// when cs has grown since the last call — avoids an O(K) allocation on every
// row for a schema that (in practice) stabilizes after the first few rows.
func ColumnCache(cs *coll.ColumnSet) func() []coll.Column {
	var cached []coll.Column
	lastLen := -1
	return func() []coll.Column {
		if cs == nil {
			return nil
		}
		if cs.Len() != lastLen {
			cached = cs.Columns()
			lastLen = cs.Len()
		}
		return cached
	}
}

// Flatten recursively flattens row's cells (named via inCols) into a fresh
// row built against outCols, e.g. inCols' "user" cell holding {b:1}
// produces outCols' "user.b" = 1.
func Flatten(inCols *coll.ColumnSet, row coll.Row, outCols *coll.ColumnSet) coll.Row {
	if inCols == nil {
		return nil
	}
	return flattenRow(inCols.Columns(), row, outCols)
}

// flattenRow is Flatten's per-row core, taking a pre-fetched column
// snapshot so callers iterating many rows against a stable schema can cache
// it (see ColumnCache) instead of paying an O(K) snapshot allocation per row.
func flattenRow(cols []coll.Column, row coll.Row, outCols *coll.ColumnSet) coll.Row {
	out := make(coll.Row, outCols.Len())
	for i, col := range cols {
		out = flattenAssign(outCols, out, col.ColumnName, row.Cell(i))
	}
	return out
}

// flattenAssign walks v, building dotted/bracketed keys under prefix and
// assigning scalars into out (grown/indexed against outCols).
func flattenAssign(outCols *coll.ColumnSet, out coll.Row, prefix string, v any) coll.Row {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			key := k
			if prefix != "" {
				key = prefix + "." + k
			}
			out = flattenAssign(outCols, out, key, val)
		}
	case []any:
		for i, val := range x {
			key := fmt.Sprintf("%s[%d]", prefix, i)
			if prefix == "" {
				key = fmt.Sprintf("[%d]", i)
			}
			out = flattenAssign(outCols, out, key, val)
		}
	default:
		if prefix != "" {
			idx := outCols.Index(prefix)
			if idx >= len(out) {
				grown := make(coll.Row, idx+1)
				copy(grown, out)
				out = grown
			}
			out[idx] = v
		}
	}
	return out
}

// Unflatten rebuilds nested structure from row's dot/bracket-named cells
// (named via inCols, the inverse of Flatten), producing a fresh row against
// outCols — always a new ColumnSet, since unflatten collapses many input
// columns into few (often one) output columns.
func Unflatten(inCols *coll.ColumnSet, row coll.Row, dropEmpty bool, outCols *coll.ColumnSet) coll.Row {
	var cols []coll.Column
	if inCols != nil {
		cols = inCols.Columns()
	}
	return UnflattenRow(cols, row, dropEmpty, outCols)
}

// unflattenRow is Unflatten's per-row core, taking a pre-fetched column
// snapshot so callers iterating many rows against a stable schema can cache
// it (see ColumnCache) instead of paying an O(K) snapshot allocation per row.
func UnflattenRow(cols []coll.Column, row coll.Row, dropEmpty bool, outCols *coll.ColumnSet) coll.Row {
	root := make(map[string]any)
	for i, col := range cols {
		setPath(root, col.ColumnName, row.Cell(i))
	}
	if dropEmpty {
		pruneEmpty(root)
	}
	// Single top-level key wrapping a map → return inner map (swl2 compat).
	if len(root) == 1 {
		for _, val := range root {
			if m, ok := val.(map[string]any); ok {
				return coll.RowFromMap(outCols, m)
			}
		}
	}
	return coll.RowFromMap(outCols, root)
}

// setPath walks path parts and assigns value at the leaf.
func setPath(root map[string]any, path string, value any) {
	parts := splitPath(path)
	cur := root
	for i, p := range parts {
		if i == len(parts)-1 {
			cur[p.key] = value
			return
		}
		next, ok := cur[p.key]
		if !ok {
			child := map[string]any{}
			cur[p.key] = child
			cur = child
			continue
		}
		child, ok := next.(map[string]any)
		if !ok {
			child = map[string]any{}
			cur[p.key] = child
		}
		cur = child
	}
}

type pathPart struct {
	key string
}

// splitPath parses "a.b[0].c" into [{a},{b},{0},{c}].
func splitPath(path string) []pathPart {
	var parts []pathPart
	var b strings.Builder
	for i := 0; i < len(path); i++ {
		switch path[i] {
		case '.':
			if b.Len() > 0 {
				parts = append(parts, pathPart{key: b.String()})
				b.Reset()
			}
		case '[':
			if b.Len() > 0 {
				parts = append(parts, pathPart{key: b.String()})
				b.Reset()
			}
			j := i + 1
			for j < len(path) && path[j] != ']' {
				j++
			}
			parts = append(parts, pathPart{key: path[i+1 : j]})
			i = j
		default:
			b.WriteByte(path[i])
		}
	}
	if b.Len() > 0 {
		parts = append(parts, pathPart{key: b.String()})
	}
	return parts
}

// pruneEmpty removes nil and empty nested maps (for -n / --no-empty).
func pruneEmpty(m map[string]any) {
	for k, v := range m {
		switch x := v.(type) {
		case map[string]any:
			pruneEmpty(x)
			if len(x) == 0 {
				delete(m, k)
			}
		case nil:
			delete(m, k)
		}
	}
}
