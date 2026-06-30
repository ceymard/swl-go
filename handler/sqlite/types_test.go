package sqlite

import "testing"

func TestIsDeclaredJSONColumnType(t *testing.T) {
	tests := []struct {
		decl string
		want bool
	}{
		{"", false},
		{"INTEGER", false},
		{"TEXT", false},
		{"VARCHAR", false},
		{"JSON", true},
		{"jsonb", true},
		{"text[]", true},
		{"struct<x int>", true},
		{"union(a int, b text)", true},
		{"ENUM('a','b')", true},
	}
	for _, tc := range tests {
		if got := isDeclaredJSONColumnType(tc.decl); got != tc.want {
			t.Fatalf("isDeclaredJSONColumnType(%q) = %v, want %v", tc.decl, got, tc.want)
		}
	}
}

func TestParseJSONCellValue(t *testing.T) {
	arr, ok := parseJSONCellValue(`["alpha","beta"]`).([]any)
	if !ok || len(arr) != 2 {
		t.Fatalf("array parse got %T %#v", parseJSONCellValue(`["alpha","beta"]`), parseJSONCellValue(`["alpha","beta"]`))
	}
	if arr[0] != "alpha" || arr[1] != "beta" {
		t.Fatalf("array elems %#v", arr)
	}

	obj, ok := parseJSONCellValue(`{"x":1}`).(map[string]any)
	if !ok || obj["x"] != int64(1) {
		t.Fatalf("object parse got %#v", parseJSONCellValue(`{"x":1}`))
	}

	// PostgreSQL array literal text is not JSON; leave as string.
	lit := `{alpha,beta}`
	if got := parseJSONCellValue(lit); got != lit {
		t.Fatalf("pg literal got %#v", got)
	}
}
