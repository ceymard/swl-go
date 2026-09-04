// Package swltest provides helpers and fakes for swl-go tests.
// It lives under test/ so nothing here ships in the production binary.
package swltest

import (
	"testing"

	"github.com/ceymard/swl-go/handler"
	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/msg"
	"github.com/ceymard/swl-go/internal/pipeline"
	"github.com/ceymard/swl-go/internal/runner"
)

// Snapshot is one materialized collection (name + columns + all rows) for
// assertions. Columns is a snapshot of the collection's ColumnSet, in
// discovery order, paired with Rows so tests can assert by column name
// instead of hardcoding indexes.
type Snapshot struct {
	Name    string
	Columns []coll.Column
	Rows    []coll.Row
}

// Cell returns Rows[row]'s value for the named column, or nil if the
// column doesn't exist or the row hasn't grown that far.
func (s Snapshot) Cell(row int, name string) any {
	for i, c := range s.Columns {
		if c.ColumnName == name {
			return s.Rows[row].Cell(i)
		}
	}
	return nil
}

// HasColumn reports whether name was ever discovered in this collection.
func (s Snapshot) HasColumn(name string) bool {
	for _, c := range s.Columns {
		if c.ColumnName == name {
			return true
		}
	}
	return false
}

// ColumnNames returns the discovery-order column names.
func (s Snapshot) ColumnNames() []string {
	names := make([]string, len(s.Columns))
	for i, c := range s.Columns {
		names[i] = c.ColumnName
	}
	return names
}

// ToCollection rebuilds s as a coll.Collection (a fresh ColumnSet seeded
// from s.Columns in the same discovery order, Rows re-batched) — useful for
// feeding a captured Snapshot back into a sink/transform in tests.
func (s Snapshot) ToCollection() coll.Collection {
	cs := coll.NewColumnSet()
	for _, c := range s.Columns {
		cs.Index(c.ColumnName)
	}
	rows := append([]coll.Row(nil), s.Rows...)
	return coll.Collection{Name: s.Name, Columns: cs, Rows: coll.SliceRowBatches(rows)}
}

// CollectStream drains a stream into snapshots (used to assert transform output).
func CollectStream(t *testing.T, s coll.Stream) ([]Snapshot, error) {
	t.Helper()
	var out []Snapshot
	for c, err := range s {
		if err != nil {
			return nil, err
		}
		snap := Snapshot{Name: c.Name}
		for batch, err := range c.Rows {
			if err != nil {
				return nil, err
			}
			snap.Rows = append(snap.Rows, batch...)
		}
		if c.Columns != nil {
			snap.Columns = c.Columns.Columns()
		}
		out = append(out, snap)
	}
	return out, nil
}

// Coll builds a Collection with cols assigned indexes in that exact order
// (deterministic — no map iteration involved) and rows already positional
// against cols, one row per []any of matching or shorter length.
func Coll(name string, cols []string, rows ...[]any) coll.Collection {
	cs := coll.NewColumnSet()
	for _, c := range cols {
		cs.Index(c)
	}
	out := make([]coll.Row, len(rows))
	for i, r := range rows {
		out[i] = coll.Row(r)
	}
	return coll.Collection{Name: name, Columns: cs, Rows: coll.SliceRowBatches(out)}
}

// RunPipeline executes a parsed pipeline with the production handler registry.
func RunPipeline(t *testing.T, p pipeline.Pipeline) error {
	t.Helper()
	cfg := runner.Config{
		Messages: msg.New(p.Verbose),
		Verbose:  p.Verbose,
	}
	return runner.Run(cfg, handler.Reg, p)
}
