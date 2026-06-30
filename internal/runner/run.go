// Package runner folds pipeline stages into one coll.Stream and runs it.
package runner

import (
	"context"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/debug"
	"github.com/ceymard/swl-go/internal/errs"
	"github.com/ceymard/swl-go/internal/handlers"
	"github.com/ceymard/swl-go/internal/pipeline"
	"github.com/ceymard/swl-go/internal/stream"
)

// Config is an alias for handlers.Config (convenience for callers).
type Config = handlers.Config

// Run executes all stages in order. Returns on first error; no error packets in stream.
//
// If the pipeline has no sink stage, debug.Sink prints collections/rows to stderr
// (swl2 behavior when stdout is a TTY and no sink is piped).
func Run(cfg Config, reg handlers.Registry, p pipeline.Pipeline) error {
	if cfg.Ctx == nil {
		cfg.Ctx = context.Background()
	}

	var s coll.Stream = stream.Empty()
	for _, stage := range p.Stages {
		h, ok := reg.Get(stage.ID)
		if !ok {
			return errs.New("unknown handler: " + stage.ID)
		}
		switch stage.Kind {
		case pipeline.StageSource:
			src, ok := h.(handlers.Source)
			if !ok {
				return errs.New("handler is not a source: " + stage.ID)
			}
			part, err := src.Source(cfg.Ctx, cfg, stage.Options)
			if err != nil {
				return errs.Wrap(err, "source "+stage.ID, "handler", stage.ID)
			}
			s = stream.Concat(s, stream.CheckContext(cfg.Ctx, part))

		case pipeline.StageTransform:
			tf, ok := h.(handlers.Transform)
			if !ok {
				return errs.New("handler is not a transform: " + stage.ID)
			}
			var err error
			s, err = tf.Transform(cfg.Ctx, cfg, s, stage.Options)
			if err != nil {
				return errs.Wrap(err, "transform "+stage.ID, "handler", stage.ID)
			}
			s = stream.CheckContext(cfg.Ctx, s)
			if cfg.Passthrough {
				s = stream.TeeRows(s, func(c coll.Collection, row coll.Row) error {
					return debug.PrintRow(cfg.Verbose, c, row)
				})
			}

		case pipeline.StageSink:
			sn, ok := h.(handlers.Sink)
			if !ok {
				return errs.New("handler is not a sink: " + stage.ID)
			}
			return sn.Sink(cfg.Ctx, cfg, s, stage.Options)
		}
	}
	return debug.Sink(cfg.Verbose, s)
}
