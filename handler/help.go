package handler

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ceymard/swl-go/internal/errs"
	"github.com/ceymard/swl-go/internal/optparse"
	"github.com/ceymard/swl-go/internal/stage"
)

var optParsers = map[string]*optparse.Parser{}

// RegisterOptParser binds an optparse parser used for --help output.
func RegisterOptParser(id string, p *optparse.Parser) {
	mu.Lock()
	defer mu.Unlock()
	optParsers[id] = p
}

// HelpText returns formatted usage for a handler id.
func HelpText(id string, prog string, alias string) string {
	mu.RLock()
	p := optParsers[id]
	mu.RUnlock()
	if p == nil {
		return fmt.Sprintf("Usage: %s %s\n\n(no detailed help for %q)\n", prog, id, id)
	}
	return p.GetHelp("", helpCommand(prog, id, alias))
}

// HelpForArgv shows handler help when argv contains --help or -h (after global flags).
// Returns text, true when help was requested and should stop execution.
func HelpForArgv(argv []string, prog string) (string, bool, error) {
	seg, ok := findHelpSegment(argv)
	if !ok {
		return "", false, nil
	}
	id, err := resolveHelpHandler(seg.tokens, seg.afterColon)
	if err != nil {
		return "", true, err
	}
	alias := seg.tokens[0]
	if strings.HasPrefix(alias, "+") {
		alias = alias[1:]
	}
	return HelpText(id, prog, alias), true, nil
}

type helpSeg struct {
	tokens     []string
	afterColon bool
}

func findHelpSegment(argv []string) (helpSeg, bool) {
	if len(argv) == 0 {
		return helpSeg{}, false
	}
	helpIdx := -1
	for i, a := range argv {
		if a == "--help" || a == "-h" {
			helpIdx = i
			break
		}
	}
	if helpIdx < 0 {
		return helpSeg{}, false
	}

	cleaned := append([]string(nil), argv[:helpIdx]...)
	cleaned = append(cleaned, argv[helpIdx+1:]...)

	segs, err := splitHelpSegments(cleaned)
	if err != nil || len(segs) == 0 {
		return helpSeg{}, false
	}

	target := len(segs) - 1
	return segs[target], true
}

func splitHelpSegments(argv []string) ([]helpSeg, error) {
	var segments []helpSeg
	var current []string
	afterColon := false

	flush := func() {
		if len(current) == 0 {
			return
		}
		segments = append(segments, helpSeg{
			tokens:     append([]string(nil), current...),
			afterColon: afterColon,
		})
		current = nil
	}

	for _, tok := range argv {
		switch tok {
		case "::":
			flush()
			afterColon = true
		case "++":
			flush()
			afterColon = false
		default:
			current = append(current, tok)
		}
	}
	flush()
	return segments, nil
}

func resolveHelpHandler(tokens []string, afterColon bool) (string, error) {
	if len(tokens) == 0 {
		return "", errs.New("empty command")
	}
	first := tokens[0]
	explicitSource := strings.HasPrefix(first, "+")
	if explicitSource {
		first = first[1:]
	}

	wantSink := afterColon
	if !afterColon {
		switch {
		case explicitSource:
			wantSink = false
		case isDualAlias(first):
			wantSink = true // default sink when both src/sink exist (swl2-style help)
		default:
			wantSink = false
		}
	}

	id, _, ok := resolveHandlerName(first, wantSink)
	if !ok {
		return "", errs.New("cannot resolve handler for help: " + tokens[0])
	}
	return id, nil
}

func isDualAlias(name string) bool {
	e, ok := aliases[name]
	return ok && e.source != "" && e.sink != ""
}

func resolveHandlerName(target string, wantSink bool) (string, stage.Kind, bool) {
	if h, k, ok := ResolveAlias(target, wantSink); ok {
		return h, k, true
	}
	ext := strings.ToLower(filepath.Ext(target))
	if h, k, ok := ResolveExtension(ext, wantSink); ok {
		return h, k, true
	}
	if idx := strings.Index(target, "://"); idx > 0 {
		proto := target[:idx+3]
		if h, k, ok := ResolveProtocol(proto, wantSink); ok {
			return h, k, true
		}
	}
	if !wantSink && len(target) > 0 && (target[0] == '[' || target[0] == '{') {
		return "json-src", stage.Source, true
	}
	return "", 0, false
}

func helpCommand(prog, handlerID, alias string) string {
	if alias != "" {
		if strings.HasSuffix(handlerID, "-src") {
			return prog + " +" + alias
		}
		return prog + " " + alias
	}
	for name, e := range aliases {
		if e.sink == handlerID {
			return prog + " " + name
		}
		if e.source == handlerID {
			return prog + " +" + name
		}
	}
	base := strings.TrimSuffix(strings.TrimSuffix(handlerID, "-src"), "-sink")
	return prog + " " + base
}

// WriteHelp prints handler help to stderr (swl2 prints help to stderr).
func WriteHelp(text string) {
	fmt.Fprintln(os.Stderr, text)
}
