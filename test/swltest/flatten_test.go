package swltest_test

import (
	"testing"

	"github.com/ceymard/swl-go/handler/flatten"
	"github.com/ceymard/swl-go/internal/coll"
)

func TestFlatten(t *testing.T) {
	inCols := coll.NewColumnSet()
	row := coll.RowFromMap(inCols, map[string]any{"user": map[string]any{"name": "Bob", "age": 30}})
	outCols := coll.NewColumnSet()
	got := flatten.Flatten(inCols, row, outCols)

	nameIdx, _ := outCols.Lookup("user.name")
	ageIdx, _ := outCols.Lookup("user.age")
	if got.Cell(nameIdx) != "Bob" || got.Cell(ageIdx) != 30 {
		t.Fatalf("got %v", got)
	}
}

func TestUnflatten(t *testing.T) {
	inCols := coll.NewColumnSet()
	row := coll.RowFromMap(inCols, map[string]any{"user.name": "Bob"})
	outCols := coll.NewColumnSet()
	got := flatten.Unflatten(inCols, row, false, outCols)

	nameIdx, _ := outCols.Lookup("name")
	if got.Cell(nameIdx) != "Bob" {
		t.Fatalf("got %v", got)
	}
}
