// Package json implements json-src and json-sink (swl2 swl-json-src/sink.ts).
//
// Source reads JSON/JSON5 from a file path or inline `[`/`{` argv. Sink writes JSON via sonic.
package json

import (
	"path/filepath"
	"strings"
)

// fileIsInlineJSON reports whether the source token is literal JSON, not a path.
func fileIsInlineJSON(source string) bool {
	if source == "" {
		return false
	}
	return source[0] == '[' || source[0] == '{'
}

// defaultCollectionName derives the collection name from path or inline JSON.
func defaultCollectionName(source string, inline bool) string {
	if inline {
		return "json"
	}
	base := filepath.Base(source)
	return strings.TrimSuffix(base, filepath.Ext(base))
}
