package optparse

import (
	"fmt"
	"sort"
	"strings"
)

var helpFlag = Flag("-h", "--help").As("__help")

// ErrHelp is returned when --help is passed to Parse.
var ErrHelp = &HelpError{}

// HelpError signals that usage should be printed.
type HelpError struct {
	Parser *Parser
}

func (e *HelpError) Error() string { return "help requested" }

func (e *HelpError) Is(target error) bool {
	_, ok := target.(*HelpError)
	return ok
}

// GetHelp formats usage text (swl2 getHelp()).
func (p *Parser) GetHelp(indent, cmd string) string {
	var b strings.Builder
	log := func(s string) { b.WriteString(s); b.WriteByte('\n') }

	groups := map[string][]*Handler{}
	var others []*Handler
	for _, h := range p.handlers {
		if h.group != "" {
			groups[h.group] = append(groups[h.group], h)
			continue
		}
		others = append(others, h)
		if len(h.Activators) == 0 && h.Key != "" {
			cmd += fmt.Sprintf(" [%s%s]", h.Key, repeatSuffix(h))
		}
	}
	log(cmd)
	log("")

	disp := func(h *Handler) {
		if len(h.Activators) > 0 {
			line := indent + "  " + strings.Join(h.Activators, " ")
			if h.help != "" {
				line += "  " + h.help
			}
			log(line)
			return
		}
		line := fmt.Sprintf("%s  [%s%s]", indent, h.Key, repeatSuffix(h))
		if h.help != "" {
			line += "  " + h.help
		}
		log(line)
		for _, sub := range h.Bases {
			log(sub.GetHelp(indent+"  ", fmt.Sprintf("where %s:", h.Key)))
		}
	}

	for _, h := range others {
		disp(h)
	}
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		log("")
		log(name)
		for _, h := range groups[name] {
			disp(h)
		}
	}
	log("")
	log(indent + "  -h, --help  Show this help")
	return b.String()
}

func repeatSuffix(h *Handler) string {
	if h.Repeating {
		return "..."
	}
	return ""
}

func scanHandlers(p *Parser) []*Handler {
	return append([]*Handler{helpFlag}, p.handlers...)
}
