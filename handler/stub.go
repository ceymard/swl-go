package handler

import (
	"context"
	"fmt"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/errs"
	"github.com/ceymard/swl-go/internal/handlers"
)

// stubHandler fails at run time for handlers not yet ported from swl2.
type stubHandler struct{ id string }

// stubOptions preserves argv for future parse-on-run if needed.
type stubOptions struct {
	id     string
	target string
	tail   []string
}

func (s stubHandler) Source(ctx context.Context, cfg handlers.Config, opts any) (coll.Stream, error) {
	return nil, errs.New(fmt.Sprintf("handler %q is not implemented yet", s.id))
}

func (s stubHandler) Transform(ctx context.Context, cfg handlers.Config, in coll.Stream, opts any) (coll.Stream, error) {
	return nil, errs.New(fmt.Sprintf("handler %q is not implemented yet", s.id))
}

func (s stubHandler) Sink(ctx context.Context, cfg handlers.Config, in coll.Stream, opts any) error {
	return errs.New(fmt.Sprintf("handler %q is not implemented yet", s.id))
}
