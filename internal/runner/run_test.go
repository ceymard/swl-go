package runner_test

import (
	"context"
	"path/filepath"
	"testing"

	_ "github.com/ceymard/swl-go/handler" // register production handlers
	"github.com/ceymard/swl-go/handler"
	"github.com/ceymard/swl-go/handler/csv"
	"github.com/ceymard/swl-go/handler/json"
	"github.com/ceymard/swl-go/internal/handlers"
	"github.com/ceymard/swl-go/internal/pipeline"
	"github.com/ceymard/swl-go/internal/runner"
	"github.com/ceymard/swl-go/internal/stage"
	"github.com/ceymard/swl-go/test/swltest"
)

func TestRunChainedSources(t *testing.T) {
	swltest.RegisterHandlers()

	var snaps []swltest.Snapshot
	handler.Register(swltest.CollectSinkID, swltest.RowCollectSink{Snaps: &snaps}, handler.Meta{})
	handler.RegisterParser(swltest.CollectSinkID, func(_ string, _ []string) (any, error) {
		return struct{}{}, nil
	})

	jsonPath := filepath.Join("..", "..", "testdata", "json", "simple.json")
	csvPath := filepath.Join("..", "..", "testdata", "csv", "simple.csv")
	jsonOpts, err := json.ParseSrcOptions(jsonPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	csvOpts, err := csv.ParseSrcOptions(csvPath, nil)
	if err != nil {
		t.Fatal(err)
	}

	p := pipeline.Pipeline{
		Stages: []pipeline.Stage{
			{Kind: stage.Source, ID: "json-src", Options: jsonOpts},
			{Kind: stage.Source, ID: "csv-src", Options: csvOpts},
			{Kind: stage.Sink, ID: swltest.CollectSinkID},
		},
	}
	cfg := handlers.Config{Ctx: context.Background()}
	if err := runner.Run(cfg, handler.Reg, p); err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, s := range snaps {
		total += len(s.Rows)
	}
	if total < 3 {
		t.Fatalf("expected rows from json + csv, got %d: %+v", total, snaps)
	}
}

func TestRunJSONSourceOnlyDebugSink(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "json", "simple.json")
	opts, err := json.ParseSrcOptions(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	p := pipeline.Pipeline{
		Verbose: 0,
		Stages: []pipeline.Stage{
			{Kind: stage.Source, ID: "json-src", Options: opts},
		},
	}
	cfg := handlers.Config{Ctx: context.Background(), Verbose: 0}
	if err := runner.Run(cfg, handler.Reg, p); err != nil {
		t.Fatal(err)
	}
}

func TestRunUnknownHandler(t *testing.T) {
	p := pipeline.Pipeline{
		Stages: []pipeline.Stage{{Kind: stage.Source, ID: "missing-handler"}},
	}
	err := runner.Run(handlers.Config{Ctx: context.Background()}, handler.Reg, p)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRunWrongHandlerKind(t *testing.T) {
	p := pipeline.Pipeline{
		Stages: []pipeline.Stage{{Kind: stage.Sink, ID: "json-src"}},
	}
	err := runner.Run(handlers.Config{Ctx: context.Background()}, handler.Reg, p)
	if err == nil {
		t.Fatal("expected error")
	}
}
