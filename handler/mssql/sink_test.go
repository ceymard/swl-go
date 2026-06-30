package mssql

import "testing"

func TestSplitTable(t *testing.T) {
	schema, table := splitTable("dbo.users")
	if schema != "dbo" || table != "users" {
		t.Fatalf("got %q %q", schema, table)
	}
	schema, table = splitTable("users")
	if schema != "dbo" || table != "users" {
		t.Fatalf("got %q %q", schema, table)
	}
}

func TestQuoteTable(t *testing.T) {
	if got := quoteTable("dbo.users"); got != "[dbo].[users]" {
		t.Fatalf("got %q", got)
	}
}

func TestBuildMergeStatement(t *testing.T) {
	stmt := buildMergeStatement("dbo.items", []string{"id", "name"}, []string{"id"},
		[]string{"[id]", "[name]"}, []string{"(@p1, @p2)"})
	if !containsAll(stmt, "MERGE", "WHEN MATCHED", "WHEN NOT MATCHED", "[dbo].[items]") {
		t.Fatalf("merge stmt %q", stmt)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !contains(s, p) {
			return false
		}
	}
	return true
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
