package coll_test

import (
	"testing"

	"github.com/ceymard/swl-go/internal/coll"
)

// TestRowFromMapSparsePadding is the correctness core of the positional-row
// design: a column discovered by an earlier row but absent from a later row
// must appear as an explicit nil at its assigned index, not be silently
// omitted (which would shift every later column's meaning).
func TestRowFromMapSparsePadding(t *testing.T) {
	cs := coll.NewColumnSet()

	row1 := coll.RowFromMap(cs, map[string]any{"a": 1, "b": 2})
	row2 := coll.RowFromMap(cs, map[string]any{"a": 10, "c": 30})

	if got, want := cs.Len(), 3; got != want {
		t.Fatalf("ColumnSet.Len() = %d, want %d", got, want)
	}
	// "a" and "b" are discovered together from the same map (row1), so Go's
	// randomized map iteration means their relative index isn't fixed by
	// this test — only that "c" (discovered later, alone) gets whichever
	// index wasn't taken by a/b, i.e. 2.
	aIdx, _ := cs.Lookup("a")
	bIdx, _ := cs.Lookup("b")
	cIdx, _ := cs.Lookup("c")
	if cIdx != 2 {
		t.Fatalf("c index = %d, want 2 (discovered after a and b)", cIdx)
	}
	if (aIdx != 0 && aIdx != 1) || (bIdx != 0 && bIdx != 1) || aIdx == bIdx {
		t.Fatalf("a/b indexes = %d,%d, want a permutation of {0,1}", aIdx, bIdx)
	}

	if got := row1.Cell(aIdx); got != 1 {
		t.Fatalf("row1.Cell(a) = %#v, want 1", got)
	}
	if got := row1.Cell(bIdx); got != 2 {
		t.Fatalf("row1.Cell(b) = %#v, want 2", got)
	}
	if len(row1) != 2 {
		t.Fatalf("len(row1) = %d, want 2 (built before c existed)", len(row1))
	}

	if got := row2.Cell(aIdx); got != 10 {
		t.Fatalf("row2.Cell(a) = %#v, want 10", got)
	}
	if got := row2.Cell(cIdx); got != 30 {
		t.Fatalf("row2.Cell(c) = %#v, want 30", got)
	}
	// The correctness core: row2 knows about "b" (an earlier-discovered
	// column absent from its own map) only via nil-padding up to c's index,
	// not by silently shifting c into b's slot.
	if got := row2.Cell(bIdx); got != nil {
		t.Fatalf("row2.Cell(b) = %#v, want nil (b known, absent from row2)", got)
	}
	if len(row2) != 3 {
		t.Fatalf("len(row2) = %d, want 3 (nil-padded through c's index)", len(row2))
	}

	// Cell() must read row1 (built before "c" existed) as nil for c's index,
	// not panic or wrap around.
	if got := row1.Cell(cIdx); got != nil {
		t.Fatalf("row1.Cell(c) = %#v, want nil", got)
	}
}

func TestColumnSetIndexStable(t *testing.T) {
	cs := coll.NewColumnSet()
	first := cs.Index("x")
	second := cs.Index("x")
	if first != second {
		t.Fatalf("Index(x) not stable: %d then %d", first, second)
	}
	cs.Index("y")
	third := cs.Index("x")
	if third != first {
		t.Fatalf("Index(x) changed after a later column was added: %d then %d", first, third)
	}
}
