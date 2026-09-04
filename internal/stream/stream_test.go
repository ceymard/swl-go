package stream_test

import (
	"testing"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/stream"
)

func TestConcatOrder(t *testing.T) {
	a := stream.Of(coll.Collection{Name: "a", Rows: coll.SliceRowBatches([]coll.Row{{1}})})
	b := stream.Of(coll.Collection{Name: "b", Rows: coll.SliceRowBatches([]coll.Row{{2}})})
	var names []string
	for c, err := range stream.Concat(a, b) {
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, c.Name)
	}
	if len(names) != 2 || names[0] != "a" || names[1] != "b" {
		t.Fatalf("got %v", names)
	}
}

func TestConcatForwardsCollectionError(t *testing.T) {
	sentinel := errSentinel{}
	a := func(yield func(coll.Collection, error) bool) {
		yield(coll.Collection{}, sentinel)
	}
	b := stream.Of(coll.Collection{Name: "b", Rows: coll.SliceRowBatches([]coll.Row{{2}})})

	var gotErr error
	var sawB bool
	for c, err := range stream.Concat(a, b) {
		if err != nil {
			gotErr = err
			continue
		}
		if c.Name == "b" {
			sawB = true
		}
	}
	if gotErr != sentinel {
		t.Fatalf("got err %v, want %v", gotErr, sentinel)
	}
	if sawB {
		t.Fatal("Concat kept iterating into b after a errored")
	}
}

func TestMapRows(t *testing.T) {
	in := stream.Of(coll.Collection{
		Name: "t",
		Rows: coll.SliceRowBatches([]coll.Row{{1}}),
	})
	out := stream.MapRows(in, func(c coll.Collection) (*coll.ColumnSet, func(coll.Row) (coll.Row, error)) {
		return c.Columns, func(row coll.Row) (coll.Row, error) {
			return coll.Row{row.Cell(0)}, nil
		}
	})
	for c, err := range out {
		if err != nil {
			t.Fatal(err)
		}
		for batch, err := range c.Rows {
			if err != nil {
				t.Fatal(err)
			}
			for _, row := range batch {
				if row.Cell(0) != 1 {
					t.Fatalf("got %v", row)
				}
			}
		}
	}
}

func TestMapRowsErrorStops(t *testing.T) {
	in := stream.Of(coll.Collection{
		Name: "t",
		Rows: coll.SliceRowBatches([]coll.Row{{1}, {2}}),
	})
	sentinel := errSentinel{}
	out := stream.MapRows(in, func(c coll.Collection) (*coll.ColumnSet, func(coll.Row) (coll.Row, error)) {
		return c.Columns, func(row coll.Row) (coll.Row, error) {
			if row.Cell(0) == 2 {
				return nil, sentinel
			}
			return row, nil
		}
	})
	for c, err := range out {
		if err != nil {
			t.Fatal(err)
		}
		for _, err := range c.Rows {
			if err == sentinel {
				return
			}
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	t.Fatal("expected error")
}

type errSentinel struct{}

func (errSentinel) Error() string { return "stop" }
