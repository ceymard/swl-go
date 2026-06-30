// Package handlers defines the interfaces each pipeline stage implements.
//
// Lives in its own package so handler/ implementations and internal/runner
// can both import it without an import cycle.
package handlers

import (
	"context"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/msg"
	"github.com/ceymard/swl-go/internal/progress"
)

// Config is passed to every handler. Messages go to stderr, not through the stream.
type Config struct {
	Ctx         context.Context
	Messages    *msg.Log
	Progress    *progress.Handler // set by runner for source/sink stages
	Verbose     int
	Passthrough bool // -p: tee rows to debug while processing
}

// Log writes handler diagnostics with role-colored prefix when Progress is set.
func (cfg Config) Log(level int, args ...any) {
	if cfg.Progress != nil {
		cfg.Progress.Log(level, args...)
		return
	}
	if cfg.Messages != nil {
		cfg.Messages.Log(level, args...)
	}
}

// ConnTarget returns a URI or path highlighted for the current source/sink role.
func (cfg Config) ConnTarget(s string) string {
	if cfg.Progress != nil {
		return cfg.Progress.Highlight(s)
	}
	return s
}

// Source emits a stream of collections (file read, DB query, …).
type Source interface {
	Source(ctx context.Context, cfg Config, opts any) (coll.Stream, error)
}

// Transform wraps an upstream stream (flatten, coerce, …).
type Transform interface {
	Transform(ctx context.Context, cfg Config, in coll.Stream, opts any) (coll.Stream, error)
}

// Sink consumes a stream terminally (write file, insert DB, …).
type Sink interface {
	Sink(ctx context.Context, cfg Config, in coll.Stream, opts any) error
}

// RowWriter handles rows for one collection after Open.
type RowWriter interface {
	Write(row coll.Row) error
	Close() error
}

// SinkHooks is the shared sink driver API (see runner.ConsumeHooks).
// Open receives the first row so sqlite can infer DDL — matches swl2 behavior.
type SinkHooks interface {
	Init(ctx context.Context) error
	Open(ctx context.Context, col coll.Collection, firstRow coll.Row) (RowWriter, error)
	Rollback(ctx context.Context)
	Finish(ctx context.Context) error
}

// Registry looks up a registered handler by id ("json-src", "flatten", …).
type Registry interface {
	Get(id string) (any, bool)
}
