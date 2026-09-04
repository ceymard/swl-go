package swltest

import (
	"context"

	"github.com/ceymard/swl-go/handler"
	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/handlers"
	"github.com/ceymard/swl-go/internal/stream"
)

const (
	MemSrcID      = "mem-src"       // in-memory source handler id for tests
	CollectSinkID = "collect-sink"  // appends rows to a *[]coll.Row
)

// MemSource yields predefined collections (test fake source).
type MemSource struct {
	Collections []coll.Collection // default when MemOptions is empty
}

type MemOptions struct {
	Collections []coll.Collection // override per pipeline segment
}

func (s MemSource) Source(ctx context.Context, cfg handlers.Config, raw any) (coll.Stream, error) {
	opts, ok := raw.(MemOptions)
	if ok && len(opts.Collections) > 0 {
		return stream.Of(opts.Collections...), nil
	}
	return stream.Of(s.Collections...), nil
}

// RowCollectSink appends one Snapshot per collection seen to Snaps (test
// fake sink) — pairs each collection's rows with its discovery-order
// columns so tests can assert by column name via Snapshot.Cell instead of
// hardcoding indexes.
type RowCollectSink struct {
	Snaps *[]Snapshot
}

func (c RowCollectSink) Sink(ctx context.Context, cfg handlers.Config, in coll.Stream, opts any) error {
	for col, err := range in {
		if err != nil {
			return err
		}
		snap := Snapshot{Name: col.Name}
		for batch, err := range col.Rows {
			if err != nil {
				return err
			}
			snap.Rows = append(snap.Rows, batch...)
		}
		if col.Columns != nil {
			snap.Columns = col.Columns.Columns()
		}
		*c.Snaps = append(*c.Snaps, snap)
	}
	return nil
}

// RegisterHandlers registers mem-src and collect-sink fakes on the global registry.
func RegisterHandlers() {
	handler.Register(MemSrcID, MemSource{}, handler.Meta{})
	handler.RegisterParser(MemSrcID, func(_ string, _ []string) (any, error) {
		return MemOptions{}, nil
	})
}
