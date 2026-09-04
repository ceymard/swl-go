package pg

import (
	"reflect"
	"testing"

	"github.com/ceymard/swl-go/internal/coll"
)

func TestTransformHstoreColumns(t *testing.T) {
	cols := []string{"id", "meta"}
	row := coll.Row{
		int64(1),
		map[string]any{
			"role": "admin",
			"team": "ops",
		},
	}
	out := transformHstoreColumns(row, cols, []string{"meta"})
	s, ok := out.Cell(1).(string)
	if !ok {
		t.Fatalf("meta type %T", out.Cell(1))
	}
	if s != `"role"=>"admin","team"=>"ops"` {
		t.Fatalf("hstore %q", s)
	}
}

func TestTransformHstoreSkipsString(t *testing.T) {
	cols := []string{"meta"}
	row := coll.Row{`"a"=>"b"`}
	out := transformHstoreColumns(row, cols, []string{"meta"})
	if out.Cell(0) != `"a"=>"b"` {
		t.Fatalf("got %+v", out.Cell(0))
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
