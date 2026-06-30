package csv

import (
	"regexp"
	"strings"

	"golang.org/x/text/unicode/norm"
)

var (
	collectionSuffixRE = regexp.MustCompile(`([-_]\d+)?\.[^.]*$`)
	simplifyHeaderRE   = regexp.MustCompile(`([^\w])+`)
)

// collectionName derives a collection name from a CSV file path (swl2 basename rule).
func collectionName(path string, override *string) string {
	if override != nil && *override != "" {
		return *override
	}
	base := path
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		base = path[i+1:]
	}
	return collectionSuffixRE.ReplaceAllString(base, "")
}

// simplifyHeader normalizes a CSV header (-s / --simplify-headers).
func simplifyHeader(header string) string {
	if header == "" {
		return ""
	}
	s := strings.ToLower(header)
	s = norm.NFD.String(s)
	var b strings.Builder
	for _, r := range s {
		if r < 0x0300 || r > 0x036f { // drop combining marks
			b.WriteRune(r)
		}
	}
	s = simplifyHeaderRE.ReplaceAllString(b.String(), "_")
	s = strings.Trim(s, "_")
	return strings.TrimSpace(s)
}

func splitList(s string) []string {
	if s == "" {
		return nil
	}
	s = strings.ReplaceAll(s, "\n", " ")
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func mergeColumns(spec string) map[string]any {
	cols := splitList(spec)
	if len(cols) == 0 {
		return nil
	}
	m := make(map[string]any, len(cols))
	for _, c := range cols {
		m[c] = nil
	}
	return m
}
