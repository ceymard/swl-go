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

	"github.com/ceymard/swl-go/internal/cli"
	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/handlers"
	"github.com/ceymard/swl-go/internal/stream"
)

type Transform struct{}

type Options struct {
	Only []string // -o: limit columns to coerce
	cli.BaseOpts
}

type optsParser struct {
	Only *string `parser:"( ( '-o' | '--only-columns' ) @Arg )?"`
	cli.BaseOpts
}

func ParseOptions(argv []string) (any, error) {
	p, err := cli.BuildParser[optsParser]()
	if err != nil {
		return nil, err
	}
	o, err := cli.ParseArgs(p, argv)
	if err != nil {
		return nil, err
	}
	opts := Options{BaseOpts: o.BaseOpts}
	if o.Only != nil {
		opts.Only = splitCSV(*o.Only)
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
		b, err := json.Marshal(v)
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
	cli.BaseOpts
}

type uncoerceParser struct {
	Only      *string `parser:"( ( '-o' | '--only-columns' ) @Arg )?"`
	Except    *string `parser:"( ( '-e' | '--except' ) @Arg )?"`
	Bool      bool    `parser:"( '-b' | '--boolean' )?"`
	Trim      bool    `parser:"( '-t' | '--trim' )?"`
	NullEmpty bool    `parser:"( '-n' | '--empty-is-null' )?"`
	cli.BaseOpts
}

func ParseUncoerceOptions(argv []string) (any, error) {
	p, err := cli.BuildParser[uncoerceParser]()
	if err != nil {
		return nil, err
	}
	o, err := cli.ParseArgs(p, argv)
	if err != nil {
		return nil, err
	}
	opts := UncoerceOptions{
		Bool: o.Bool, Trim: o.Trim, NullEmpty: o.NullEmpty, BaseOpts: o.BaseOpts,
	}
	if o.Only != nil {
		opts.Only = splitCSV(*o.Only)
	}
	if o.Except != nil {
		opts.Except = splitCSV(*o.Except)
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
		if err := json.Unmarshal([]byte(s), &v); err == nil {
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
