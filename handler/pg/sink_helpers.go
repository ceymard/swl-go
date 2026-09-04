package pg

import (
	"fmt"
	"sort"
	"strings"

	"github.com/ceymard/swl-go/internal/coll"
)

type pgIndex struct {
	Name    string
	Schema  string
	Def     string
}

func tempTableName(collection string) string {
	return strings.ReplaceAll(collection, ".", "__") + "_temp"
}

// tableRowTypeRef is the composite type passed to json_populate_record (swl2: null::${table}).
func tableRowTypeRef(collection, defaultSchema string) string {
	if strings.Contains(collection, ".") {
		return collection
	}
	schema, table := qualifyTable(collection, defaultSchema)
	if schema == "public" {
		return table
	}
	return schema + "." + table
}

// columnNames snapshots col's columns at Open time, in natural discovery
// order (no sort — see plan's "Sink output order"). Columns appearing in
// rows after this snapshot are silently dropped, matching prior behavior.
func columnNames(col coll.Collection) []string {
	if col.Columns == nil {
		return nil
	}
	cs := col.Columns.Columns()
	names := make([]string, len(cs))
	for i, c := range cs {
		names[i] = c.ColumnName
	}
	return names
}

func quoteColumns(cols []string) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = `"` + c + `"`
	}
	return out
}

func containsString(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func parseFQTable(fqTable string) (schema, table string) {
	clean := strings.ReplaceAll(fqTable, `"`, "")
	parts := strings.Split(clean, ".")
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "public", clean
}

func buildCreateTable(fqTable string, cols []string) string {
	parts := make([]string, len(cols))
	for i, c := range cols {
		parts[i] = fmt.Sprintf(`"%s" text`, c)
	}
	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (%s)`, fqTable, strings.Join(parts, ", "))
}

func hstoreQuote(s string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s)
}

func formatHstore(m map[string]any) string {
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	sort.Strings(names)
	parts := make([]string, len(names))
	for i, name := range names {
		val := fmt.Sprint(m[name])
		parts[i] = fmt.Sprintf(`"%s"=>"%s"`, hstoreQuote(name), hstoreQuote(val))
	}
	return strings.Join(parts, ",")
}

// transformHstoreColumns rewrites any hstoreCols cell holding a map value
// into its hstore text representation. cols gives the name for each row
// position (both are the same Open-time snapshot, so cols[i] is row[i]'s name).
func transformHstoreColumns(row coll.Row, cols, hstoreCols []string) coll.Row {
	if len(hstoreCols) == 0 {
		return row
	}
	out := make(coll.Row, len(row))
	copy(out, row)
	for _, col := range hstoreCols {
		i := indexOfString(cols, col)
		if i < 0 {
			continue
		}
		v := row.Cell(i)
		if v == nil {
			continue
		}
		if _, isStr := v.(string); isStr {
			continue
		}
		if m, ok := v.(map[string]any); ok {
			out[i] = formatHstore(m)
		}
	}
	return out
}

func indexOfString(ss []string, want string) int {
	for i, s := range ss {
		if s == want {
			return i
		}
	}
	return -1
}
