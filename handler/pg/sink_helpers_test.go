package pg

import (
	"reflect"
	"testing"

	"github.com/ceymard/swl-go/internal/coll"
)

func TestTransformHstoreColumns(t *testing.T) {
	row := coll.Row{
		"id": int64(1),
		"meta": map[string]any{
			"role": "admin",
			"team": "ops",
		},
	}
	out := transformHstoreColumns(row, []string{"meta"})
	s, ok := out["meta"].(string)
	if !ok {
		t.Fatalf("meta type %T", out["meta"])
	}
	if s != `"role"=>"admin","team"=>"ops"` {
		t.Fatalf("hstore %q", s)
	}
}

func TestTransformHstoreSkipsString(t *testing.T) {
	row := coll.Row{"meta": `"a"=>"b"`}
	out := transformHstoreColumns(row, []string{"meta"})
	if out["meta"] != `"a"=>"b"` {
		t.Fatalf("got %+v", out["meta"])
	}
}

func TestTableRowTypeRef(t *testing.T) {
	if got := tableRowTypeRef("app.users", "public"); got != "app.users" {
		t.Fatalf("got %q", got)
	}
	if got := tableRowTypeRef("users", "public"); got != "users" {
		t.Fatalf("got %q", got)
	}
	if got := tableRowTypeRef("items", "app"); got != "app.items" {
		t.Fatalf("got %q", got)
	}
}

func TestTempTableName(t *testing.T) {
	if got := tempTableName("app.users"); got != "app__users_temp" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatHstoreDeterministic(t *testing.T) {
	a := formatHstore(map[string]any{"b": "2", "a": "1"})
	b := formatHstore(map[string]any{"a": "1", "b": "2"})
	if a != b {
		t.Fatalf("%q != %q", a, b)
	}
	if !reflect.DeepEqual(a, `"a"=>"1","b"=>"2"`) {
		t.Fatalf("got %q", a)
	}
}
