// Package swltest provides helpers and fakes for swl-go tests.
// It lives under test/ so nothing here ships in the production binary.
package swltest

import (
	"testing"

	"github.com/ceymard/swl-go/handler"
	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/msg"
	"github.com/ceymard/swl-go/internal/pipeline"
	"github.com/ceymard/swl-go/internal/runner"
)

// Snapshot is one materialized collection (name + all rows) for assertions.
type Snapshot struct {
	Name string
	Rows []coll.Row
}

// CollectStream drains a stream into snapshots (used to assert transform output).
func CollectStream(t *testing.T, s coll.Stream) ([]Snapshot, error) {
	t.Helper()
	var out []Snapshot
	for c, err := range s {
		if err != nil {
			return nil, err
		}
		snap := Snapshot{Name: c.Name}
		for batch, err := range c.Rows {
			if err != nil {
				return nil, err
			}
			snap.Rows = append(snap.Rows, batch...)
		}
		out = append(out, snap)
	}
	return out, nil
}

// RunPipeline executes a parsed pipeline with the production handler registry.
func RunPipeline(t *testing.T, p pipeline.Pipeline) error {
	t.Helper()
	cfg := runner.Config{
		Messages: msg.New(p.Verbose),
		Verbose:  p.Verbose,
	}
	return runner.Run(cfg, handler.Reg, p)
}
