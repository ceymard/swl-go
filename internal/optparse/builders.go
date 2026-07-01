package optparse

import "strings"

// Flag defines a boolean flag (swl2 flag().as()).
func Flag(activators ...string) *Handler {
	if len(activators) == 0 {
		panic("optparse: flag requires activators")
	}
	h := &Handler{Activators: append([]string(nil), activators...)}
	h.scan = func(args []string, pos int, acc [][]string) ([]string, error) {
		if pos >= len(args) {
			return nil, nil
		}
		for _, a := range h.Activators {
			if args[pos] == a {
				return []string{args[pos]}, nil
			}
		}
		return nil, nil
	}
	h.value = func(groups [][]string, _ *parseCtx) (any, error) {
		if h.Repeating {
			return len(groups), nil
		}
		if len(groups) > 1 {
			return nil, matchErr(strings.Join(h.Activators, " ") + " can only appear once")
		}
		return len(groups) > 0, nil
	}
	return h
}

// Param defines a flag with an optional value (swl2 param().as()).
func Param(activators ...string) *Handler {
	if len(activators) == 0 {
		panic("optparse: param requires activators")
	}
	h := &Handler{Activators: append([]string(nil), activators...)}
	h.scan = func(args []string, pos int, acc [][]string) ([]string, error) {
		if pos >= len(args) {
			return nil, nil
		}
		for _, a := range h.Activators {
			if args[pos] != a {
				continue
			}
			end := pos + 1
			if end < len(args) && !strings.HasPrefix(args[end], "-") {
				end++
			}
			return args[pos:end], nil
		}
		return nil, nil
	}
	h.value = func(groups [][]string, _ *parseCtx) (any, error) {
		if h.Repeating {
			var out []string
			for _, g := range groups {
				if len(g) > 1 {
					out = append(out, g[1])
				}
			}
			return out, nil
		}
		if len(groups) > 1 {
			return nil, matchErr(strings.Join(h.Activators, " ") + " can only appear once")
		}
		if len(groups) == 0 {
			return nil, nil
		}
		if len(groups[0]) > 1 {
			return groups[0][1], nil
		}
		return nil, nil
	}
	return h
}

// Arg defines a positional argument (swl2 arg()).
func Arg(key string) *Handler {
	h := &Handler{Key: key}
	h.scan = func(args []string, pos int, acc [][]string) ([]string, error) {
		if !h.Repeating && len(acc) > 0 {
			return nil, nil
		}
		if pos >= len(args) {
			return nil, nil
		}
		if strings.HasPrefix(args[pos], "-") {
			return nil, nil
		}
		return []string{args[pos]}, nil
	}
	h.value = func(groups [][]string, _ *parseCtx) (any, error) {
		if h.Repeating {
			var out []string
			for _, g := range groups {
				if len(g) > 0 {
					out = append(out, g[0])
				}
			}
			return out, nil
		}
		if len(groups) == 0 || len(groups[0]) == 0 {
			return nil, nil
		}
		return groups[0][0], nil
	}
	return h
}

// Expect matches a literal token (swl2 expect().as()).
func Expect(value string) *Handler {
	h := &Handler{Key: value, Activators: []string{value}}
	h.scan = func(args []string, pos int, acc [][]string) ([]string, error) {
		if len(acc) > 0 || pos >= len(args) {
			return nil, nil
		}
		if args[pos] != value {
			return nil, matchErr(`expected "` + value + `"`)
		}
		return []string{args[pos]}, nil
	}
	h.value = func(_ [][]string, _ *parseCtx) (any, error) {
		return value, nil
	}
	return h
}

// Oneof tries sub-parsers in order (swl2 oneof().as()).
func Oneof(parsers ...*Parser) *Handler {
	h := &Handler{Bases: parsers}
	h.scan = nil // handled in doScan
	h.value = func(groups [][]string, ctx *parseCtx) (any, error) {
		single := func(g []string) (any, error) {
			key := oneofKey(g)
			entry, ok := ctx.oneof[key]
			if !ok {
				return nil, nil
			}
			return entry.parser.doValues(entry.mapres, ctx)
		}
		if h.Repeating {
			var out []any
			for _, g := range groups {
				v, err := single(g)
				if err != nil {
					return nil, err
				}
				if v != nil {
					out = append(out, v)
				}
			}
			return out, nil
		}
		if len(groups) > 1 {
			return nil, matchErr("oneof internal error")
		}
		if len(groups) == 0 {
			return nil, nil
		}
		return single(groups[0])
	}
	return h
}

// As sets the result key (swl2 .as()).
// Oneof handlers keep positional keys only — swl2 does not add --key activators for oneof.
func (h *Handler) As(key string) *Handler {
	h.Key = key
	if len(h.Activators) == 0 && len(h.Bases) == 0 {
		h.Activators = []string{"--" + key}
	}
	return h
}

// Required marks the handler value as mandatory (swl2 .required()).
func (h *Handler) Required() *Handler {
	h.required = true
	return h
}

// Default sets the value when the handler did not match (swl2 .default()).
func (h *Handler) Default(v any) *Handler {
	h.DefaultVal = v
	h.HasDefault = true
	return h
}

// Repeat allows multiple matches (swl2 .repeat()).
func (h *Handler) Repeat() *Handler {
	h.Repeating = true
	return h
}

// Help sets help text (swl2 .help()).
func (h *Handler) Help(text string) *Handler {
	h.help = text
	return h
}

// Group sets a help group name (swl2 .group()).
func (h *Handler) Group(name string) *Handler {
	h.group = name
	return h
}
