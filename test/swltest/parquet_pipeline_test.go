package swltest_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/ceymard/swl-go/handler"
	"github.com/ceymard/swl-go/internal/handlers"
	"github.com/ceymard/swl-go/internal/pipeline"
	"github.com/ceymard/swl-go/internal/runner"
	"github.com/ceymard/swl-go/internal/stage"
	"github.com/ceymard/swl-go/test/swltest"
)

func TestJSONToParquetPipeline(t *testing.T) {
	swltest.RegisterHandlers()

	jsonPath := filepath.Join("..", "..", "testdata", "json", "simple.json")
	outPath := filepath.Join(t.TempDir(), "people.parquet")

	p, err := pipeline.Parse([]string{jsonPath, "::", outPath}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Stages) != 2 || p.Stages[0].ID != "json-src" || p.Stages[1].ID != "parquet-sink" {
		t.Fatalf("stages %+v", p.Stages)
	}

	cfg := handlers.Config{Ctx: context.Background(), Verbose: 0}
	if err := runner.Run(cfg, handler.Reg, p); err != nil {
		t.Fatal(err)
	}

	var snaps []swltest.Snapshot
	handler.Register(swltest.CollectSinkID, swltest.RowCollectSink{Snaps: &snaps}, handler.Meta{})
	handler.RegisterParser(swltest.CollectSinkID, func(_ string, _ []string) (any, error) {
		return struct{}{}, nil
	})

	p2, err := pipeline.Parse([]string{outPath}, 0)
	if err != nil {
		t.Fatal(err)
	}
	p2.Stages = append(p2.Stages, pipeline.Stage{Kind: stage.Sink, ID: swltest.CollectSinkID})
	if err := runner.Run(cfg, handler.Reg, p2); err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, s := range snaps {
		total += len(s.Rows)
	}
	if total != 2 {
		t.Fatalf("rows %d", total)
	}
}
