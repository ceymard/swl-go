// Package unflatten implements the unflatten transform (swl2 swl-unflatten.ts).
//
// Rebuilds nested maps from flat dot/bracket keys produced by flatten.
package unflatten

import (
	"context"

	"github.com/ceymard/swl-go/handler/flatten"
	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/handlers"
	"github.com/ceymard/swl-go/internal/optparse"
	"github.com/ceymard/swl-go/internal/stream"
)

type Transform struct{}

type Options struct {
	NoEmpty bool
}

var optsParser = optparse.Optparser(
	optparse.Flag("-n", "--no-empty").As("noempty"),
)

func ParseOptions(argv []string) (any, error) {
	m, err := optsParser.Parse(argv)
	if err != nil {
		return nil, err
	}
	return Options{NoEmpty: optparse.Bool(m, "noempty")}, nil
}

func (Transform) Transform(ctx context.Context, cfg handlers.Config, in coll.Stream, raw any) (coll.Stream, error) {
	opts := raw.(Options)
	return stream.MapRows(in, func(row coll.Row) (coll.Row, error) {
		return flatten.Unflatten(row, opts.NoEmpty), nil
	}), nil
}
