// Package coerce implements coerce/uncoerce transforms (swl2 swl-coerce.ts).
//
// Coerce stringifies values for DB/CSV sinks; Uncoerce parses strings back to
// numbers, booleans, dates, and JSON blobs on read paths.
package coerce

import (
	"context"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/handlers"
	"github.com/ceymard/swl-go/internal/jsonx"
	"github.com/ceymard/swl-go/internal/optparse"
	"github.com/ceymard/swl-go/internal/stream"
)

type Transform struct{}

type Options struct {
	Only []string // -o: limit columns to coerce
}

var coerceParser = optparse.Optparser(
	optparse.Param("-o", "--only-columns").As("only").Help("Comma-separated columns to coerce (default: all)"),
)

// OptParser returns the optparse parser for coerce.
func OptParser() *optparse.Parser { return coerceParser }

func ParseOptions(argv []string) (any, error) {
	m, err := coerceParser.Parse(argv)
	if err != nil {
		return nil, err
	}
	opts := Options{}
	if only := optparse.Str(m, "only"); only != "" {
		opts.Only = splitCSV(only)
	}
	return opts, nil
}

func (Transform) Transform(ctx context.Context, cfg handlers.Config, in coll.Stream, raw any) (coll.Stream, error) {
	opts := raw.(Options)
	only := toSet(opts.Only)
	return stream.MapRows(in, func(row coll.Row) (coll.Row, error) {
		out := make(coll.Row, len(row))
		for k, v := range row {
			if only != nil && !only[k] {
				out[k] = v
				continue
			}
			out[k] = Coerce(v)
		}
		return out, nil
	}), nil
}

// Coerce converts a cell value to a string-friendly form for sinks.
func Coerce(value any) any {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case string, float64, int, int64, json.Number:
		return value // already scalar
	case bool:
		if v {
			return "true"
		}
		return "false"
	case time.Time:
		return v.UTC().Format(time.RFC3339Nano)
	default:
		// objects/arrays → JSON string
		b, err := jsonx.Marshal(v)
		if err != nil {
			return value
		}
		return string(b)
	}
}

type UncoerceTransform struct{}

type UncoerceOptions struct {
	Only      []string
	Except    []string
	Bool      bool // -b: parse true/false strings
	Trim      bool // -t: trim whitespace on strings
	NullEmpty bool // -n: "" → nil
}

var uncoerceParser = optparse.Optparser(
	optparse.Param("-o", "--only-columns").As("only").Help("Comma-separated columns to uncoerce"),
	optparse.Param("-e", "--except").As("except").Help("Comma-separated columns to skip"),
	optparse.Flag("-b", "--boolean").As("boolean").Help("Parse true/false strings as booleans"),
	optparse.Flag("-t", "--trim").As("trim").Help("Trim whitespace from strings"),
	optparse.Flag("-n", "--empty-is-null").As("empty_is_null").Help("Treat empty strings as null"),
)

// OptParser returns the optparse parser for uncoerce.
func UncoerceOptParser() *optparse.Parser { return uncoerceParser }

func ParseUncoerceOptions(argv []string) (any, error) {
	m, err := uncoerceParser.Parse(argv)
	if err != nil {
		return nil, err
	}
	opts := UncoerceOptions{
		Bool:      optparse.Bool(m, "boolean"),
		Trim:      optparse.Bool(m, "trim"),
		NullEmpty: optparse.Bool(m, "empty_is_null"),
	}
	if only := optparse.Str(m, "only"); only != "" {
		opts.Only = splitCSV(only)
	}
	if except := optparse.Str(m, "except"); except != "" {
		opts.Except = splitCSV(except)
	}
	return opts, nil
}

func (UncoerceTransform) Transform(ctx context.Context, cfg handlers.Config, in coll.Stream, raw any) (coll.Stream, error) {
	opts := raw.(UncoerceOptions)
	only := toSet(opts.Only)
	except := toSet(opts.Except)
	return stream.MapRows(in, func(row coll.Row) (coll.Row, error) {
		out := make(coll.Row, len(row))
		for k, v := range row {
			if only != nil && !only[k] {
				out[k] = v
				continue
			}
			if except != nil && except[k] {
				out[k] = v
				continue
			}
			out[k] = Uncoerce(v, opts)
		}
		return out, nil
	}), nil
}

// Patterns for heuristic string parsing (ported from swl2).
var (
	reDate    = regexp.MustCompile(`(?i)^\d{4}-\d{2}-\d{2}(?:T\d{2}:\d{2}(?::\d{2}(?:\.\d{3}Z?)?)?)?$`)
	reNumber  = regexp.MustCompile(`(?i)^\d+(\.\d+)?$`)
	reBoolean = regexp.MustCompile(`(?i)^true|false$`)
	reNull    = regexp.MustCompile(`(?i)^null$`)
)

// Uncoerce tries to parse a string cell back to a typed value.
func Uncoerce(value any, opts UncoerceOptions) any {
	s, ok := value.(string)
	if !ok {
		return value
	}
	// JSON object/array literal
	if len(s) > 0 && (s[0] == '{' || s[0] == '[') {
		var v any
		if err := jsonx.Unmarshal([]byte(s), &v); err == nil {
			return v
		}
	}
	trimmed := strings.TrimSpace(s)
	if opts.NullEmpty && trimmed == "" {
		return nil
	}
	if reDate.MatchString(trimmed) {
		if t, err := time.Parse(time.RFC3339Nano, trimmed); err == nil {
			return t
		}
		if t, err := time.Parse("2006-01-02", trimmed); err == nil {
			return t
		}
	}
	if opts.Bool && reBoolean.MatchString(trimmed) {
		return strings.EqualFold(trimmed, "true")
	}
	if reNumber.MatchString(trimmed) {
		if f, err := strconv.ParseFloat(trimmed, 64); err == nil {
			return f
		}
	}
	if reNull.MatchString(trimmed) {
		return nil
	}
	if opts.Trim {
		return trimmed
	}
	return value
}

func splitCSV(s string) []string {
	return regexp.MustCompile(`[\n\s]*,[\s\n]*`).Split(s, -1)
}

func toSet(list []string) map[string]bool {
	if list == nil {
		return nil
	}
	m := make(map[string]bool, len(list))
	for _, s := range list {
		m[s] = true
	}
	return m
}
