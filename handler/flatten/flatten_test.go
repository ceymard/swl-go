package flatten_test

import (
	"testing"

	"github.com/ceymard/swl-go/handler/flatten"
	"github.com/ceymard/swl-go/internal/coll"
)

// cellOf reads row's cell for name via cs, or nil if name was never seen.
func cellOf(cs *coll.ColumnSet, row coll.Row, name string) any {
	idx, ok := cs.Lookup(name)
	if !ok {
		return nil
	}
	return row.Cell(idx)
}

func TestFlattenNestedMap(t *testing.T) {
	inCols := coll.NewColumnSet()
	row := coll.RowFromMap(inCols, map[string]any{
		"user": map[string]any{"name": "Ann", "age": 30},
	})
	outCols := coll.NewColumnSet()
	got := flatten.Flatten(inCols, row, outCols)
	if cellOf(outCols, got, "user.name") != "Ann" || cellOf(outCols, got, "user.age") != 30 {
		t.Fatalf("got %+v", got)
	}
}

func TestFlattenArray(t *testing.T) {
	inCols := coll.NewColumnSet()
	row := coll.RowFromMap(inCols, map[string]any{
		"tags": []any{"a", "b"},
	})
	outCols := coll.NewColumnSet()
	got := flatten.Flatten(inCols, row, outCols)
	if cellOf(outCols, got, "tags[0]") != "a" || cellOf(outCols, got, "tags[1]") != "b" {
		t.Fatalf("got %+v", got)
	}
}

func TestUnflattenRoundTrip(t *testing.T) {
	inCols := coll.NewColumnSet()
	row := coll.RowFromMap(inCols, map[string]any{
		"x": map[string]any{"y": 1},
	})
	flatCols := coll.NewColumnSet()
	flat := flatten.Flatten(inCols, row, flatCols)

	backCols := coll.NewColumnSet()
	back := flatten.Unflatten(flatCols, flat, false, backCols)
	if cellOf(backCols, back, "y") != 1 {
		t.Fatalf("got %+v", back)
	}
}

func TestUnflattenDropEmpty(t *testing.T) {
	inCols := coll.NewColumnSet()
	row := coll.RowFromMap(inCols, map[string]any{
		"a.b": "",
		"a.c": 1,
	})
	outCols := coll.NewColumnSet()
	got := flatten.Unflatten(inCols, row, true, outCols)
	if cellOf(outCols, got, "c") != 1 {
		t.Fatalf("got %+v", got)
	}
	if idx, ok := outCols.Lookup("b"); ok {
		if v := got.Cell(idx); v != "" && v != nil {
			t.Fatalf("expected empty b dropped or nil, got %v", v)
		}
	}
}
