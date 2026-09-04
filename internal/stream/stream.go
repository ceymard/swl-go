// Package stream provides combinators over coll.Stream (concat, map, tee).
package stream

import (
	"context"

	"github.com/ceymard/swl-go/internal/coll"
)

// Empty returns a stream that yields nothing (initial state before first source).
func Empty() coll.Stream {
	return func(yield func(coll.Collection, error) bool) {}
}

// Concat yields all collections from a, then all from b.
// Matches swl2 chained sources (++) where upstream is forwarded before the next source runs.
func Concat(a, b coll.Stream) coll.Stream {
	return func(yield func(coll.Collection, error) bool) {
		for c, err := range a {
			if err != nil {
				yield(c, err)
				return
			}
			if !yield(c, nil) {
				return
			}
		}
		for c, err := range b {
			if err != nil {
				yield(c, err)
				return
			}
			if !yield(c, nil) {
				return
			}
		}
	}
}

// Of builds a stream from a fixed list of collections (tests, small sources).
func Of(collections ...coll.Collection) coll.Stream {
	return func(yield func(coll.Collection, error) bool) {
		for _, c := range collections {
			if !yield(c, nil) {
				return
			}
		}
	}
}

// MapRows applies a per-collection row mapper built by build. build runs
// once per collection and returns the output ColumnSet (pass the input
// Collection's Columns through unchanged for a transform that preserves the
// key space, e.g. coerce; return a fresh *coll.ColumnSet for one that
// renames/restructures keys, e.g. flatten) plus the row-mapping function.
// Used by transforms (flatten, coerce, …).
func MapRows(in coll.Stream, build func(c coll.Collection) (outCols *coll.ColumnSet, fn func(coll.Row) (coll.Row, error))) coll.Stream {
	return func(yield func(coll.Collection, error) bool) {
		for c, err := range in {
			if err != nil {
				yield(coll.Collection{}, err)
				return
			}
			outCols, fn := build(c)
			out := coll.Collection{
				Name:    c.Name,
				Columns: outCols,
				Rows: func(yield func([]coll.Row, error) bool) {
					for batch, err := range c.Rows {
						if err != nil {
							yield(nil, err)
							return
						}
						nb := make([]coll.Row, len(batch))
						for i, row := range batch {
							nr, err := fn(row)
							if err != nil {
								yield(nil, err)
								return
							}
							nb[i] = nr
						}
						if !yield(nb, nil) {
							return
						}
					}
				},
			}
			if !yield(out, nil) {
				return
			}
		}
	}
}

// TeeRows calls side for each row while forwarding downstream.
// Implements swl2 -p passthrough (print/debug tap without altering the stream).
func TeeRows(in coll.Stream, side func(coll.Collection, coll.Row) error) coll.Stream {
	return func(yield func(coll.Collection, error) bool) {
		for c, err := range in {
			if err != nil {
				yield(coll.Collection{}, err)
				return
			}
			out := coll.Collection{
				Name:    c.Name,
				Columns: c.Columns,
				Rows: func(yield func([]coll.Row, error) bool) {
					for batch, err := range c.Rows {
						if err != nil {
							yield(nil, err)
							return
						}
						if side != nil {
							for _, row := range batch {
								if err := side(c, row); err != nil {
									yield(nil, err)
									return
								}
							}
						}
						if !yield(batch, nil) {
							return
						}
					}
				},
			}
			if !yield(out, nil) {
				return
			}
		}
	}
}

// CheckContext aborts iteration when ctx is cancelled (long-running sqlite/csv reads).
func CheckContext(ctx context.Context, in coll.Stream) coll.Stream {
	if ctx == nil {
		return in
	}
	return func(yield func(coll.Collection, error) bool) {
		for c, err := range in {
			if err := ctx.Err(); err != nil {
				yield(coll.Collection{}, err)
				return
			}
			if err != nil {
				yield(coll.Collection{}, err)
				return
			}
			out := coll.Collection{
				Name:    c.Name,
				Columns: c.Columns,
				Rows: func(yield func([]coll.Row, error) bool) {
					for batch, err := range c.Rows {
						if err := ctx.Err(); err != nil {
							yield(nil, err)
							return
						}
						if err != nil {
							yield(nil, err)
							return
						}
						if !yield(batch, nil) {
							return
						}
					}
				},
			}
			if !yield(out, nil) {
				return
			}
		}
	}
}
