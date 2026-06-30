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
	return stream.MapRows(in, func(row coll.Row) (coll.Row, error) {
		return Flatten(row), nil
	}), nil
}

// Flatten recursively flattens nested map[string]any and []any values into one Row.
func Flatten(row coll.Row) coll.Row {
	flat := make(coll.Row)
	flattenMap("", row, flat)
	return flat
}

// flattenMap walks v, building dotted/bracketed keys under prefix.
func flattenMap(prefix string, v any, out coll.Row) {
	switch x := v.(type) {
	case map[string]any:
		for k, val := range x {
			key := k
			if prefix != "" {
				key = prefix + "." + k
			}
			flattenMap(key, val, out)
		}
	case []any:
		for i, val := range x {
			key := fmt.Sprintf("%s[%d]", prefix, i)
			if prefix == "" {
				key = fmt.Sprintf("[%d]", i)
			}
			flattenMap(key, val, out)
		}
	default:
		if prefix != "" {
			out[prefix] = v
		}
	}
}

// Unflatten rebuilds nested structure from dot/bracket keys (inverse of Flatten).
func Unflatten(row coll.Row, dropEmpty bool) coll.Row {
	root := make(map[string]any)
	for k, v := range row {
		setPath(root, k, v)
	}
	if dropEmpty {
		pruneEmpty(root)
	}
	// Single top-level key wrapping a map → return inner map (swl2 compat).
	if len(root) == 1 {
		for _, val := range root {
			if m, ok := val.(map[string]any); ok {
				return m
			}
		}
	}
	out := make(coll.Row)
	for k, v := range root {
		out[k] = v
	}
	return out
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
