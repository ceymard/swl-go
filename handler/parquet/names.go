package parquet

import (
	"regexp"
	"strings"

	"github.com/ceymard/swl-go/internal/coll"
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

func projectRow(row coll.Row, cols []string) coll.Row {
	if len(cols) == 0 {
		return row
	}
	out := make(coll.Row, len(cols))
	for _, c := range cols {
		out[c] = row[c]
	}
	return out
}
