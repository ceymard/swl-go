// Package unflatten implements the unflatten transform (swl2 swl-unflatten.ts).
//
// Rebuilds nested maps from flat dot/bracket keys produced by flatten.
package unflatten

import (
	"context"

	"github.com/ceymard/swl-go/internal/cli"
	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/handler/flatten"
	"github.com/ceymard/swl-go/internal/handlers"
	"github.com/ceymard/swl-go/internal/stream"
)

type Transform struct{}

type Options struct {
	NoEmpty bool // -n: drop empty branches after rebuild
	cli.BaseOpts
}

type optsParser struct {
	NoEmpty bool `parser:"( '-n' | '--no-empty' )?"`
	cli.BaseOpts
}

func ParseOptions(argv []string) (any, error) {
	p, err := cli.BuildParser[optsParser]()
	if err != nil {
		return nil, err
	}
	o, err := cli.ParseArgs(p, argv)
	if err != nil {
		return nil, err
	}
	return Options{NoEmpty: o.NoEmpty, BaseOpts: o.BaseOpts}, nil
}

func (Transform) Transform(ctx context.Context, cfg handlers.Config, in coll.Stream, raw any) (coll.Stream, error) {
	opts := raw.(Options)
	return stream.MapRows(in, func(row coll.Row) (coll.Row, error) {
		return flatten.Unflatten(row, opts.NoEmpty), nil
	}), nil
}
