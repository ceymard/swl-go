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
			if err != nil || !yield(c, err) {
				return
			}
		}
		for c, err := range b {
			if err != nil || !yield(c, err) {
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

// MapRows applies fn to each row; collection name and columns pass through unchanged.
// Used by transforms (flatten, coerce, …).
func MapRows(in coll.Stream, fn func(coll.Row) (coll.Row, error)) coll.Stream {
	return func(yield func(coll.Collection, error) bool) {
		for c, err := range in {
			if err != nil {
				yield(coll.Collection{}, err)
				return
			}
			out := coll.Collection{
				Name:    c.Name,
				Columns: c.Columns,
				Rows: func(yield func(coll.Row, error) bool) {
					for row, err := range c.Rows {
						if err != nil {
							yield(nil, err)
							return
						}
						nr, err := fn(row)
						if err != nil {
							yield(nil, err)
							return
						}
						if !yield(nr, nil) {
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
				Rows: func(yield func(coll.Row, error) bool) {
					for row, err := range c.Rows {
						if err != nil {
							yield(nil, err)
							return
						}
						if side != nil {
							if err := side(c, row); err != nil {
								yield(nil, err)
								return
							}
						}
						if !yield(row, nil) {
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
				Rows: func(yield func(coll.Row, error) bool) {
					for row, err := range c.Rows {
						if err := ctx.Err(); err != nil {
							yield(nil, err)
							return
						}
						if err != nil {
							yield(nil, err)
							return
						}
						if !yield(row, nil) {
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
