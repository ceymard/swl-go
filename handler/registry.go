// Package handler registers all pipeline stages and resolves CLI names to handler ids.
//
// Aliases/extensions/protocols mirror swl2/scripts/swl.ts. Handlers register via
// init() in register.go; stubs fail at run time with a clear error.
package handler

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/ceymard/swl-go/internal/stage"
	"github.com/ceymard/swl-go/internal/style"
)

// Handler is any registered stage implementation (Source, Transform, or Sink).
type Handler any

// Meta describes how a handler appears in the registry.
type Meta struct {
	TransformOnly bool // flatten, coerce, … — not a file/db sink
	Stub          bool // not implemented yet; fails in Run
}

var (
	mu       sync.RWMutex
	byID     = map[string]Handler{}
	metaByID = map[string]Meta{}
	parsers  = map[string]func(target string, tail []string) (any, error){}
)

// Register adds a handler implementation. Called from init() in register.go.
func Register(id string, h Handler, m Meta) {
	mu.Lock()
	defer mu.Unlock()
	byID[id] = h
	metaByID[id] = m
}

// RegisterParser binds an optparse argv parser for a handler id.
func RegisterParser(id string, fn func(target string, tail []string) (any, error)) {
	mu.Lock()
	defer mu.Unlock()
	parsers[id] = fn
}

// Get returns the handler for id.
func Get(id string) (Handler, bool) {
	mu.RLock()
	defer mu.RUnlock()
	h, ok := byID[id]
	return h, ok
}

func IsTransformOnly(id string) bool {
	mu.RLock()
	defer mu.RUnlock()
	return metaByID[id].TransformOnly
}

func IsStub(id string) bool {
	mu.RLock()
	defer mu.RUnlock()
	return metaByID[id].Stub
}

// ParseOptions parses handler-specific flags. target is the path/URI (or inline JSON);
// tail holds remaining argv tokens (flags). Transforms ignore target.
func ParseOptions(id string, target string, tail []string) (any, error) {
	if IsStub(id) {
		return stubOptions{id: id, target: target, tail: tail}, nil
	}
	p, ok := parsers[id]
	if !ok {
		return struct{}{}, nil // handler with no flags (flatten)
	}
	return p(target, tail)
}

// aliasEntry maps a user-facing name to source and/or sink handler ids.
type aliasEntry struct {
	source string
	sink   string
}

// aliases, extensions, protocols ported from swl2/scripts/swl.ts.
var (
	aliases = map[string]aliasEntry{
		"pg": {"pg-src", "pg-sink"}, "postgres": {"pg-src", "pg-sink"},
		"mysql": {"my-src", ""}, "my": {"my-src", ""},
		"sqlite": {"sqlite-src", "sqlite-sink"},
		"duckdb": {"duckdb-src", "duckdb-sink"},
		"xl": {"xlsx-src", "xlsx-sink"}, "xls": {"xlsx-src", "xlsx-sink"},
		"xlsx": {"xlsx-src", "xlsx-sink"},
		"yaml": {"yaml-src", "yaml-sink"}, "yml": {"yaml-src", "yaml-sink"},
		"json": {"json-src", "json-sink"}, "csv": {"csv-src", "csv-sink"},
		"parquet": {"parquet-src", "parquet-sink"}, "pqt": {"parquet-src", "parquet-sink"},
		"fn": {"", "fn"},
		"flatten": {"", "flatten"}, "unflatten": {"", "unflatten"},
		"coerce": {"", "coerce"}, "uncoerce": {"", "uncoerce"},
	}

	extensions = map[string]aliasEntry{
		".csv": {"csv-src", "csv-sink"},
		".pqt": {"parquet-src", ""}, ".parquet": {"parquet-src", ""},
		".db": {"sqlite-src", "sqlite-sink"}, ".sqlite": {"sqlite-src", "sqlite-sink"},
		".ddb": {"duckdb-src", "duckdb-sink"}, ".duckdb": {"duckdb-src", "duckdb-sink"},
		".xlsx": {"xlsx-src", "xlsx-sink"}, ".ods": {"xlsx-src", "xlsx-sink"},
		".xlsb": {"xlsx-src", "xlsx-sink"}, ".xls": {"xlsx-src", "xlsx-sink"},
		".xlsm": {"xlsx-src", "xlsx-sink"},
		".yaml": {"yaml-src", "yaml-sink"}, ".yml": {"yaml-src", "yaml-sink"},
		".json": {"json-src", "json-sink"},
	}

	protocols = map[string]aliasEntry{
		"postgres://": {"pg-src", "pg-sink"},
		"mysql://":    {"my-src", ""},
	}
)

// SplitSourcePrefix strips a leading + that marks an explicit source handler (+pg → pg).
func SplitSourcePrefix(token string) (name string, explicitSource bool) {
	if strings.HasPrefix(token, "+") && len(token) > 1 {
		return token[1:], true
	}
	return token, false
}

func ResolveAlias(name string, wantSink bool) (id string, kind stage.Kind, ok bool) {
	e, ok := aliases[name]
	if !ok {
		return "", 0, false
	}
	return pickEntry(e, wantSink)
}

func ResolveExtension(ext string, wantSink bool) (id string, kind stage.Kind, ok bool) {
	e, ok := extensions[ext]
	if !ok {
		return "", 0, false
	}
	return pickEntry(e, wantSink)
}

func ResolveProtocol(proto string, wantSink bool) (id string, kind stage.Kind, ok bool) {
	e, ok := protocols[proto]
	if !ok {
		return "", 0, false
	}
	return pickEntry(e, wantSink)
}

// pickEntry chooses source vs sink handler id for an alias entry.
func pickEntry(e aliasEntry, wantSink bool) (string, stage.Kind, bool) {
	if wantSink {
		if e.sink == "" {
			// mysql alias: sink side falls back to source handler id used as sink
			if e.source != "" {
				return e.source, stage.Sink, true
			}
			return "", 0, false
		}
		kind := stage.Sink
		if IsTransformOnly(e.sink) {
			kind = stage.Transform
		}
		return e.sink, kind, true
	}
	if e.source == "" {
		return "", 0, false
	}
	return e.source, stage.Source, true
}

// ListAvailable prints handlers, extensions, and protocols (swl2 empty-command output).
func ListAvailable() string {
	var b strings.Builder
	writeAliasSection(&b, "handlers:\n", aliases)
	writeAliasSection(&b, "extensions:\n", extensions)
	writeAliasSection(&b, "protocols:\n", protocols)
	return b.String()
}

// WriteAvailable writes the handler list to w, with colors when w is a TTY.
func WriteAvailable(w io.Writer) {
	colorize := style.Enabled(w)
	if !colorize {
		fmt.Fprint(w, ListAvailable())
		return
	}
	writeAliasSectionStyled(w, style.Heading("handlers:", true), aliases, colorize)
	writeAliasSectionStyled(w, style.Heading("extensions:", true), extensions, colorize)
	writeAliasSectionStyled(w, style.Heading("protocols:", true), protocols, colorize)
}

func writeAliasSectionStyled(w io.Writer, heading string, m map[string]aliasEntry, colorize bool) {
	fmt.Fprintln(w, heading)
	for _, name := range sortedKeys(m) {
		sig := aliasSigil(m[name])
		switch sig {
		case "⇄":
			sig = style.SigilBoth(sig, colorize)
		case "←":
			sig = style.SigilSource(sig, colorize)
		default:
			sig = style.SigilSink(sig, colorize)
		}
		fmt.Fprintf(w, "  %s %s\n", sig, style.HandlerName(name, colorize))
	}
	fmt.Fprintln(w)
}

func writeAliasSection(b *strings.Builder, heading string, m map[string]aliasEntry) {
	b.WriteString(heading)
	for _, name := range sortedKeys(m) {
		fmt.Fprintf(b, "  %s %s\n", aliasSigil(m[name]), name)
	}
	b.WriteByte('\n')
}

func sortedKeys(m map[string]aliasEntry) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// aliasSigil matches swl2 show_aliases: ⇄ both, ← source, → sink.
func aliasSigil(e aliasEntry) string {
	if e.source != "" && e.sink != "" {
		return "⇄"
	}
	if e.source != "" {
		return "←"
	}
	return "→"
}

// ListAliases formats handler names only (legacy helper).
func ListAliases() string {
	var b strings.Builder
	writeAliasSection(&b, "handlers:\n", aliases)
	return strings.TrimSuffix(b.String(), "\n") + "\n"
}
