package xlsx_test

import "github.com/ceymard/swl-go/internal/coll"

// rowsFromMaps builds positional rows sharing one ColumnSet, from map
// literals — a test-only convenience for constructing fixture collections.
func rowsFromMaps(maps []map[string]any) ([]coll.Row, *coll.ColumnSet) {
	cs := coll.NewColumnSet()
	rows := make([]coll.Row, len(maps))
	for i, m := range maps {
		rows[i] = coll.RowFromMap(cs, m)
	}
	return rows, cs
}
