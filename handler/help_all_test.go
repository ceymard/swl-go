package handler_test

import (
	"strings"
	"testing"

	"github.com/ceymard/swl-go/handler"
)

// Dual handlers: one --help shows both source and sink sections.
var dualHelpCases = []struct {
	argv      []string
	srcHint   string
	sinkHint  string
}{
	{[]string{"json", "--help"}, "JSON file path", "Output file, directory"},
	{[]string{"csv", "--help"}, "Field delimiter", "Omit header row"},
	{[]string{"yaml", "--help"}, "!!e eval tags", "Output .yml file"},
	{[]string{"parquet", "--help"}, "[selections...]", "Output file, directory"},
	{[]string{"xlsx", "--help"}, "Spreadsheet file", "Output spreadsheet path"},
	{[]string{"duckdb", "--help"}, "DuckDB database file", "Per-collection sink options"},
	{[]string{"pg", "--help"}, "--schema", "--auto-create"},
	{[]string{"my", "--help"}, "MySQL connection URI", "Per-collection sink options"},
	{[]string{"mssql", "--help"}, "Schema.table or query", "Per-collection sink options"},
	{[]string{"sqlite", "--help"}, "SQLite database file", "Truncate table before load"},
}

func TestHelpForAllDualHandlers(t *testing.T) {
	for _, tc := range dualHelpCases {
		text, ok, err := handler.HelpForArgv(tc.argv, "swl")
		if err != nil {
			t.Errorf("%v: err=%v", tc.argv, err)
			continue
		}
		if !ok {
			t.Errorf("%v: help not shown", tc.argv)
			continue
		}
		if !strings.Contains(text, "SOURCE\n") || !strings.Contains(text, "SINK\n") {
			t.Errorf("%v: missing SOURCE/SINK sections\n%s", tc.argv, text)
			continue
		}
		if !strings.Contains(text, tc.srcHint) {
			t.Errorf("%v: source help missing %q\n%s", tc.argv, tc.srcHint, text)
		}
		if !strings.Contains(text, tc.sinkHint) {
			t.Errorf("%v: sink help missing %q\n%s", tc.argv, tc.sinkHint, text)
		}
		if !strings.Contains(text, "++") || !strings.Contains(text, "::") {
			t.Errorf("%v: missing ++ or :: in usage\n%s", tc.argv, text)
		}
	}
}

var transformHelpCases = []struct {
	argv    []string
	contain string
}{
	{[]string{"coerce", "--help"}, "only-columns"},
	{[]string{"unflatten", "--help"}, "no-empty"},
	{[]string{"uncoerce", "--help"}, "empty-is-null"},
}

func TestHelpForTransformHandlers(t *testing.T) {
	for _, tc := range transformHelpCases {
		text, ok, err := handler.HelpForArgv(tc.argv, "swl")
		if err != nil || !ok {
			t.Fatalf("%v: ok=%v err=%v", tc.argv, ok, err)
		}
		if strings.Contains(text, "SOURCE\n") {
			t.Fatalf("%v: transforms should not have dual help:\n%s", tc.argv, text)
		}
		if !strings.Contains(text, tc.contain) {
			t.Fatalf("%v: help missing %q\n%s", tc.argv, tc.contain, text)
		}
	}
}

func TestHelpForArgvParquetNestedColumns(t *testing.T) {
	text, ok, err := handler.HelpForArgv([]string{"parquet", "--help"}, "swl")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if !strings.Contains(text, "where selections: <options>") || !strings.Contains(text, "--columns") {
		t.Fatalf("expected nested parquet selections help, got:\n%s", text)
	}
}
