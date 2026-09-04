package parquet

import (
	"regexp"
	"strings"
)

// swl2 swl-parquet-src.ts: basename(file).replace(/(-\d*)?\.[^\.]*$/, "")
var collectionSuffixRE = regexp.MustCompile(`(-\d*)?\.[^.]*$`)

func collectionName(path string) string {
	base := path
	if i := strings.LastIndexAny(path, `/\`); i >= 0 {
		base = path[i+1:]
	}
	return collectionSuffixRE.ReplaceAllString(base, "")
}

func parseColumns(spec string) []string {
	if strings.TrimSpace(spec) == "" {
		return nil
	}
	parts := strings.Split(spec, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// projectMap selects cols (in the given order) out of v, which is the
// parquet reader's raw decoded row (expected to be map[string]any). An
// empty cols means "no projection" — v passes through unchanged.
func projectMap(v any, cols []string) any {
	if len(cols) == 0 {
		return v
	}
	m, ok := v.(map[string]any)
	if !ok {
		return v
	}
	out := make(map[string]any, len(cols))
	for _, c := range cols {
		out[c] = m[c]
	}
	return out
}
