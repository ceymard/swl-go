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

// Collection is a named set of rows. Name becomes table name / file stem / JSON key.
// Columns are optional type hints (from DB DESCRIBE etc.); nil means infer from first row.
type Collection struct {
	Name    string
	Columns []schema.Column
	Rows    iter.Seq2[Row, error]
}

// Stream yields collections in order. Chained sources (++ ) concatenate streams.
type Stream = iter.Seq2[Collection, error]

// SliceRows wraps an in-memory slice as a row iterator (json source, tests).
func SliceRows(rows []Row) iter.Seq2[Row, error] {
	return func(yield func(Row, error) bool) {
		for _, row := range rows {
			if !yield(row, nil) {
				return // consumer stopped early
			}
		}
	}
}
