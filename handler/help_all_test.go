package handler_test

import (
	"strings"
	"testing"

	"github.com/ceymard/swl-go/handler"
)

// Each entry is argv for HelpForArgv and a substring that must appear in help output.
var helpCases = []struct {
	argv    []string
	contain string
}{
	{[]string{"+json", "--help"}, "JSON file path"},
	{[]string{"json", "--help"}, "Output file, directory"},
	{[]string{"+csv", "--help"}, "Field delimiter"},
	{[]string{"csv", "--help"}, "Omit header row"},
	{[]string{"+yaml", "--help"}, "!!e eval tags"},
	{[]string{"yaml", "--help"}, "Output .yml file"},
	{[]string{"+parquet", "--help"}, "[selections...]"},
	{[]string{"parquet", "--help"}, "Output file, directory"},
	{[]string{"+xlsx", "--help"}, "Spreadsheet file"},
	{[]string{"xlsx", "--help"}, "Output spreadsheet path"},
	{[]string{"+duckdb", "--help"}, "DuckDB database file"},
	{[]string{"duckdb", "--help"}, "Per-collection sink options"},
	{[]string{"+my", "--help"}, "MySQL connection URI"},
	{[]string{"+mssql", "--help"}, "Schema.table or query"},
	{[]string{"coerce", "--help"}, "only-columns"},
	{[]string{"unflatten", "--help"}, "no-empty"},
	{[]string{"uncoerce", "--help"}, "empty-is-null"},
}

func TestHelpForAllHandlers(t *testing.T) {
	for _, tc := range helpCases {
		text, ok, err := handler.HelpForArgv(tc.argv, "swl")
		if err != nil {
			t.Errorf("%v: err=%v", tc.argv, err)
			continue
		}
		if !ok {
			t.Errorf("%v: help not shown", tc.argv)
			continue
		}
		if !strings.Contains(text, tc.contain) {
			t.Errorf("%v: help missing %q\n%s", tc.argv, tc.contain, text)
		}
		if !strings.Contains(text, "-h, --help") {
			t.Errorf("%v: missing --help line", tc.argv)
		}
	}
}

func TestHelpForArgvCsvSourceDelimiter(t *testing.T) {
	text, ok, err := handler.HelpForArgv([]string{"+csv", "--help"}, "swl")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if !strings.Contains(text, "--gunzip") || !strings.Contains(text, "--simplify-headers") {
		t.Fatalf("expected csv-src flags, got:\n%s", text)
	}
}

func TestHelpForArgvJsonSinkObject(t *testing.T) {
	text, ok, err := handler.HelpForArgv([]string{"json", "--help"}, "swl")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if !strings.Contains(text, "--object") {
		t.Fatalf("expected json-sink --object, got:\n%s", text)
	}
}

func TestHelpForArgvParquetNestedColumns(t *testing.T) {
	text, ok, err := handler.HelpForArgv([]string{"+parquet", "--help"}, "swl")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if !strings.Contains(text, "where selections: <options>") || !strings.Contains(text, "--columns") {
		t.Fatalf("expected nested parquet selections help, got:\n%s", text)
	}
}
