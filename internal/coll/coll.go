// Package coll defines the core data units flowing through a pipeline.
//
// A pipeline yields collections; each collection yields batches of rows.
// Both levels use Go 1.23 iter.Seq2 pull iterators — no channels, no packet
// wrappers.
//
// A Row is a positional slice, not a map: cell i corresponds to the column
// at index i in the collection's ColumnSet. ColumnSet assigns each column
// name an index the first time it's seen (within one Collection's Rows
// stream), and that assignment is permanent — an index, once given to a
// name, never means anything else in that collection. This lets rows from
// different points in the stream have different lengths (a row built
// before column N was discovered is simply shorter) without ever becoming
// ambiguous: Row.Cell handles the "not grown that far yet" case as nil, and
// RowFromMap nil-pads for a discovered-but-absent column. All row reads
// must go through Row.Cell rather than direct indexing.
package coll

import (
	"iter"

	"github.com/ceymard/swl-go/internal/schema"
)

// Column is an alias for schema.Column, re-exported for convenience.
type Column = schema.Column

// Row is one record: cell i is the value for ColumnSet's column i. A Row
// may be shorter than its ColumnSet if it was built before later columns
// were discovered — always read via Cell, never by direct index, outside
// this package.
type Row []any

// Cell returns row[i], or nil if the row hasn't grown that far yet.
func (r Row) Cell(i int) any {
	if i < 0 || i >= len(r) {
		return nil
	}
	return r[i]
}

// DefaultBatchSize is how many rows sources/transforms accumulate per yield.
// Batching amortizes per-yield overhead (context checks, progress counters)
// across many rows instead of paying it once per row.
const DefaultBatchSize = 1024

// RowBatches is the row-level iterator: each yield delivers up to
// DefaultBatchSize rows instead of one.
type RowBatches = iter.Seq2[[]Row, error]

// ColumnSet is a collection-scoped, append-only column registry. It is
// shared by pointer across a whole Rows stream (and, for a transform that
// preserves the input key space, across that transform's output stream
// too — see stream.MapRows). Index assignment is permanent: once a name
// has an index, that index means that name for the life of the set.
type ColumnSet struct {
	cols []Column
	idx  map[string]int
}

// NewColumnSet returns an empty, ready-to-grow ColumnSet.
func NewColumnSet() *ColumnSet {
	return &ColumnSet{idx: map[string]int{}}
}

// Index returns name's index, assigning the next free index on first sight.
func (cs *ColumnSet) Index(name string) int {
	if i, ok := cs.idx[name]; ok {
		return i
	}
	i := len(cs.cols)
	cs.cols = append(cs.cols, Column{ColumnName: name})
	cs.idx[name] = i
	return i
}

// Lookup returns name's index without assigning one, and whether it exists.
func (cs *ColumnSet) Lookup(name string) (int, bool) {
	i, ok := cs.idx[name]
	return i, ok
}

// Len returns the number of columns known so far.
func (cs *ColumnSet) Len() int { return len(cs.cols) }

// Columns returns a snapshot of the columns known so far, in discovery
// order. Safe to retain — later growth of cs does not mutate it.
func (cs *ColumnSet) Columns() []Column {
	return append([]Column(nil), cs.cols...)
}

// RowFromMap builds a Row against cs from m, assigning indexes for any
// unseen keys and nil-padding so row[cs.Index(k)] == m[k] for every k,
// including columns discovered by earlier rows but absent from m.
func RowFromMap(cs *ColumnSet, m map[string]any) Row {
	row := make(Row, cs.Len())
	for k, v := range m {
		i := cs.Index(k)
		if i >= len(row) {
			grown := make(Row, i+1)
			copy(grown, row)
			row = grown
		}
		row[i] = v
	}
	return row
}

// Collection is a named set of rows. Name becomes table name / file stem / JSON key.
// Columns grows as new column names are discovered across the Rows stream.
type Collection struct {
	Name    string
	Columns *ColumnSet
	Rows    RowBatches
}

// Stream yields collections in order. Chained sources (++ ) concatenate streams.
type Stream = iter.Seq2[Collection, error]

// SliceRowBatches wraps an in-memory slice as a chunked batch iterator
// (tests, and sinks that fully materialize a collection before writing).
func SliceRowBatches(rows []Row) RowBatches {
	return func(yield func([]Row, error) bool) {
		for i := 0; i < len(rows); i += DefaultBatchSize {
			end := min(i+DefaultBatchSize, len(rows))
			if !yield(rows[i:end], nil) {
				return // consumer stopped early
			}
		}
	}
}
