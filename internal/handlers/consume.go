package handlers

import (
	"context"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/errs"
)

// ConsumeHooks drives a SinkHooks implementation over a stream.
func ConsumeHooks(cfg Config, hooks SinkHooks, in coll.Stream) (err error) {
	if cfg.Ctx == nil {
		cfg.Ctx = context.Background()
	}
	if err = hooks.Init(cfg.Ctx); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			hooks.Rollback(cfg.Ctx)
		}
		finErr := hooks.Finish(cfg.Ctx)
		if err == nil {
			err = finErr
		}
	}()

	for c, err := range in {
		if err != nil {
			return err
		}
		var w RowWriter
		var opened bool
		for row, err := range c.Rows {
			if err != nil {
				return errs.Wrap(err, "read row", "collection", c.Name)
			}
			if !opened {
				w, err = hooks.Open(cfg.Ctx, c, row)
				if err != nil {
					return errs.Wrap(err, "open collection", "collection", c.Name)
				}
				opened = true
				continue // first row consumed by Open
			}
			if err = w.Write(row); err != nil {
				return errs.Wrap(err, "write row", "collection", c.Name)
			}
		}
		if w != nil {
			if err = w.Close(); err != nil {
				return errs.Wrap(err, "close collection", "collection", c.Name)
			}
		}
	}
	return nil
}
