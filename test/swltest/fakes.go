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

// RowCollectSink appends all rows to Rows (test fake sink).
type RowCollectSink struct {
	Rows *[]coll.Row
}

func (c RowCollectSink) Sink(ctx context.Context, cfg handlers.Config, in coll.Stream, opts any) error {
	for col, err := range in {
		if err != nil {
			return err
		}
		for row, err := range col.Rows {
			if err != nil {
				return err
			}
			*c.Rows = append(*c.Rows, row)
		}
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
