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

	name, _ := SplitSourcePrefix(seg.tokens[0])

	// Dual handlers show both source and sink unless the help token is in a :: segment.
	if !seg.afterColon {
		if text, ok := combinedHelp(prog, name); ok {
			return text, true, nil
		}
		if ext := strings.ToLower(filepath.Ext(name)); ext != "" {
			if text, ok := combinedHelpForEntry(prog, extLabel(ext), lookupExtension(ext)); ok {
				return text, true, nil
			}
		}
		if proto := matchProtocol(name); proto != "" {
			if text, ok := combinedHelpForEntry(prog, proto, lookupProtocol(proto)); ok {
				return text, true, nil
			}
		}
	}

	id, err := resolveHelpHandler(seg.tokens, seg.afterColon)
	if err != nil {
		return "", true, err
	}
	return HelpText(id, prog, name), true, nil
}

func combinedHelp(prog, name string) (string, bool) {
	return combinedHelpForEntry(prog, name, lookupAlias(name))
}

func combinedHelpForEntry(prog, label string, e aliasEntry) (string, bool) {
	if e.source == "" || e.sink == "" {
		return "", false
	}
	return formatDualHelp(prog, label, e.source, e.sink), true
}

func formatDualHelp(prog, label, srcID, sinkID string) string {
	src := HelpText(srcID, prog, label)
	sink := HelpText(sinkID, prog, label)

	src = setUsageLine(src, srcUsage(prog, label))
	sink = setUsageLine(sink, sinkUsage(prog, label))
	sink = trimFromGroup(sink, "BASE SWL OPTIONS")
	src = stripHelpLines(src)
	sink = stripHelpLines(sink)

	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\n\n", prog, label)
	b.WriteString("SOURCE\n")
	b.WriteString(strings.TrimRight(src, "\n"))
	b.WriteString("\n\nSINK\n")
	b.WriteString(strings.TrimRight(sink, "\n"))
	b.WriteString("\n\n  -h, --help  Show this help\n")
	return b.String()
}

func stripHelpLines(text string) string {
	var b strings.Builder
	for line := range strings.SplitSeq(text, "\n") {
		if strings.Contains(line, "-h, --help") {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(line)
	}
	return b.String()
}

func lookupAlias(name string) aliasEntry {
	mu.RLock()
	defer mu.RUnlock()
	return aliases[name]
}

func lookupExtension(ext string) aliasEntry {
	mu.RLock()
	defer mu.RUnlock()
	return extensions[ext]
}

func lookupProtocol(proto string) aliasEntry {
	mu.RLock()
	defer mu.RUnlock()
	return protocols[proto]
}

func matchProtocol(token string) string {
	if idx := strings.Index(token, "://"); idx > 0 {
		return token[:idx+3]
	}
	return ""
}

func extLabel(ext string) string {
	if ext == "" {
		return ext
	}
	return "*" + ext
}

func srcUsage(prog, label string) string {
	return fmt.Sprintf("%s %s …  (chain sources with ++)", prog, label)
}

func sinkUsage(prog, label string) string {
	return fmt.Sprintf("%s … :: %s …", prog, label)
}

func setUsageLine(help, usage string) string {
	if i := strings.IndexByte(help, '\n'); i >= 0 {
		return usage + help[i:]
	}
	return usage + "\n"
}

func trimFromGroup(text, group string) string {
	for _, prefix := range []string{"\n\n" + group + "\n", "\n" + group + "\n"} {
		if idx := strings.Index(text, prefix); idx >= 0 {
			return strings.TrimRight(text[:idx], "\n") + "\n"
		}
	}
	return text
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
	first, explicitSource := SplitSourcePrefix(tokens[0])

	wantSink := afterColon && !explicitSource
	if !afterColon && !explicitSource && isDualAlias(first) {
		wantSink = true
	}

	id, _, ok := resolveHandlerName(first, wantSink)
	if !ok && !wantSink {
		id, _, ok = resolveHandlerName(first, true)
	}
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
	if alias == "" {
		alias = inferAlias(handlerID)
	}
	if strings.HasSuffix(handlerID, "-src") {
		return srcUsage(prog, alias)
	}
	if strings.HasSuffix(handlerID, "-sink") {
		return sinkUsage(prog, alias)
	}
	return fmt.Sprintf("%s :: %s", prog, alias)
}

func inferAlias(handlerID string) string {
	for name, e := range aliases {
		if e.sink == handlerID || e.source == handlerID {
			return name
		}
	}
	return strings.TrimSuffix(strings.TrimSuffix(handlerID, "-src"), "-sink")
}

// WriteHelp prints handler help to stderr (swl2 prints help to stderr).
func WriteHelp(text string) {
	fmt.Fprintln(os.Stderr, text)
}
