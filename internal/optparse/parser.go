package optparse

import (
	"fmt"
	"strings"

	"github.com/ceymard/swl-go/internal/errs"
)

// Optparser builds a parser from handlers (swl2 optparser()).
func Optparser(handlers ...*Handler) *Parser {
	p := &Parser{}
	return p.AddHandler(handlers...)
}

// Clone returns a copy of the parser (swl2 clone()).
func (p *Parser) Clone() *Parser {
	n := &Parser{handlers: append([]*Handler(nil), p.handlers...)}
	return n
}

// Include merges another parser's handlers (swl2 include()).
func (p *Parser) Include(other *Parser) *Parser {
	n := p.Clone()
	if other != nil {
		n.handlers = append(n.handlers, other.handlers...)
	}
	return n
}

// AddHandler appends handlers (swl2 add_handler()).
func (p *Parser) AddHandler(handlers ...*Handler) *Parser {
	n := p.Clone()
	n.handlers = append(n.handlers, handlers...)
	return n
}

// Parse expands flags, scans argv, and returns a result map (swl2 parse()).
func (p *Parser) Parse(argv []string) (map[string]any, error) {
	argv = ExpandFlags(argv)
	ctx := &parseCtx{oneof: make(map[string]oneofEntry)}
	r, err := p.doScan(argv, 0, ctx)
	if err != nil {
		return nil, err
	}
	if r.pos != len(argv) {
		return nil, errs.New("unrecognized argument '" + argv[r.pos] + "'")
	}
	return p.doValues(r.mapres, ctx)
}

func (p *Parser) doScan(args []string, pos int, ctx *parseCtx) (*scanResult, error) {
	mapres := make(map[*Handler][][]string)
	for _, h := range p.handlers {
		mapres[h] = nil
	}

	l := len(args)
	init := pos
scanargs:
	for pos < l {
		for _, h := range p.handlers {
			acc := mapres[h]
			var consumed []string
			var err error
			if len(h.Bases) > 0 {
				consumed, err = h.scanOneof(args, pos, ctx)
			} else {
				consumed, err = h.scan(args, pos, acc)
			}
			if err != nil {
				if _, ok := err.(*MatchError); ok {
					return nil, err
				}
				return nil, err
			}
			if consumed == nil {
				continue
			}
			pos += len(consumed)
			mapres[h] = append(acc, consumed)
			continue scanargs
		}
		break
	}

	if pos == init && pos < l {
		return nil, matchErr("nothing was consumed")
	}
	return &scanResult{pos: pos, mapres: mapres}, nil
}

func (h *Handler) scanOneof(args []string, pos int, ctx *parseCtx) ([]string, error) {
	var errors []string
	for _, sub := range h.Bases {
		r, err := sub.doScan(args, pos, ctx)
		if err != nil {
			if me, ok := err.(*MatchError); ok {
				errors = append(errors, me.Message)
				continue
			}
			return nil, err
		}
		consumed := args[pos:r.pos]
		ctx.oneof[oneofKey(consumed)] = oneofEntry{mapres: r.mapres, parser: sub}
		return consumed, nil
	}
	if len(errors) == 0 {
		return nil, nil
	}
	return nil, matchErr(strings.Join(errors, ", "))
}

func (p *Parser) doValues(mapres map[*Handler][][]string, ctx *parseCtx) (map[string]any, error) {
	res := make(map[string]any)
	for _, h := range p.handlers {
		groups := mapres[h]
		if groups == nil {
			groups = [][]string{}
		}
		v, err := h.value(groups, ctx)
		if err != nil {
			return nil, err
		}
		if v == nil {
			if h.required {
				label := h.Key
				if len(h.Activators) > 0 {
					label = strings.Join(h.Activators, " ")
				}
				return nil, errs.New(`"` + label + `" must be specified`)
			}
			if h.HasDefault {
				v = h.DefaultVal
			}
		}
		if h.Key != "" {
			res[h.Key] = v
		}
	}
	return res, nil
}

func oneofKey(tokens []string) string {
	return strings.Join(tokens, "\x00")
}

// Str returns a string value from a parse result.
func Str(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

// StrPtr returns a string pointer when the key is present and non-empty.
func StrPtr(m map[string]any, key string) *string {
	s, ok := m[key].(string)
	if !ok || s == "" {
		return nil
	}
	return &s
}

// Bool returns a bool value from a parse result.
func Bool(m map[string]any, key string) bool {
	v, ok := m[key].(bool)
	return ok && v
}

// Int returns an int value (used for repeated flags like -v).
func Int(m map[string]any, key string) int {
	switch v := m[key].(type) {
	case int:
		return v
	case []any:
		return len(v)
	case []bool:
		return len(v)
	default:
		return 0
	}
}

// StringSlice returns a []string from repeated args or oneof results.
func StringSlice(m map[string]any, key string) []string {
	switch v := m[key].(type) {
	case []string:
		return v
	case string:
		if v == "" {
			return nil
		}
		return []string{v}
	default:
		return nil
	}
}

// MapSlice returns a slice of nested parse results from repeated oneof.
func MapSlice(m map[string]any, key string) []map[string]any {
	raw, ok := m[key].([]any)
	if !ok {
		if one, ok := m[key].(map[string]any); ok {
			return []map[string]any{one}
		}
		return nil
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		if mm, ok := item.(map[string]any); ok {
			out = append(out, mm)
		}
	}
	return out
}

// ParseSub parses argv with a sub-parser and returns typed errors consistently.
func ParseSub(p *Parser, argv []string) (map[string]any, error) {
	m, err := p.Parse(argv)
	if err != nil {
		if me, ok := err.(*MatchError); ok {
			return nil, errs.New("match error: " + me.Message)
		}
		return nil, err
	}
	return m, nil
}

// MustKey panics when a required key is missing (internal handler use).
func MustKey(m map[string]any, key string) any {
	v, ok := m[key]
	if !ok {
		panic(fmt.Sprintf("optparse: missing key %q", key))
	}
	return v
}
