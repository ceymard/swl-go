// Package coll defines the core data units flowing through a pipeline.
//
// A pipeline yields collections; each collection yields rows. Both levels use
// Go 1.23 iter.Seq2 pull iterators — no channels, no packet wrappers.
package coll

import (
	"iter"

	"github.com/ceymard/swl-go/internal/schema"
)

// Row is one record. Sources typically emit map[string]any; sinks infer schema from keys.
type Row = map[string]any

// DefaultBatchSize is how many rows sources/transforms accumulate per yield.
// Batching amortizes per-yield overhead (context checks, progress counters)
// across many rows instead of paying it once per row.
const DefaultBatchSize = 1024

// RowBatches is the row-level iterator: each yield delivers up to
// DefaultBatchSize rows instead of one.
type RowBatches = iter.Seq2[[]Row, error]

// Collection is a named set of rows. Name becomes table name / file stem / JSON key.
// Columns are optional type hints (from DB DESCRIBE etc.); nil means infer from first row.
type Collection struct {
	Name    string
	Columns []schema.Column
	Rows    RowBatches
}

// Stream yields collections in order. Chained sources (++ ) concatenate streams.
type Stream = iter.Seq2[Collection, error]

// SliceRowBatches wraps an in-memory slice as a chunked batch iterator
// (json source, tests).
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
