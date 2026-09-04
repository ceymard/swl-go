package swltest_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ceymard/swl-go/handler"
	"github.com/ceymard/swl-go/internal/handlers"
	"github.com/ceymard/swl-go/internal/pipeline"
	"github.com/ceymard/swl-go/internal/runner"
	"github.com/ceymard/swl-go/internal/stage"
	"github.com/ceymard/swl-go/test/swltest"
)

func TestXLSXToSQLitePipeline(t *testing.T) {
	swltest.RegisterHandlers()

	xlsxPath := filepath.Join("..", "..", "testdata", "xlsx", "simple.xlsx")
	dbPath := filepath.Join(t.TempDir(), "out.db")

	p, err := pipeline.Parse([]string{xlsxPath, "::", dbPath}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Stages) != 2 || p.Stages[0].ID != "xlsx-src" || p.Stages[1].ID != "sqlite-sink" {
		t.Fatalf("stages %+v", p.Stages)
	}

	cfg := handlers.Config{Ctx: context.Background(), Verbose: 0}
	if err := runner.Run(cfg, handler.Reg, p); err != nil {
		t.Fatal(err)
	}

	// Read back via sqlite-src (auto table list)
	p2, err := pipeline.Parse([]string{dbPath}, 0)
	if err != nil {
		t.Fatal(err)
	}
	var snaps []swltest.Snapshot
	handler.Register(swltest.CollectSinkID, swltest.RowCollectSink{Snaps: &snaps}, handler.Meta{})
	handler.RegisterParser(swltest.CollectSinkID, func(_ string, _ []string) (any, error) {
		return struct{}{}, nil
	})
	p2.Stages = append(p2.Stages, pipeline.Stage{Kind: stage.Sink, ID: swltest.CollectSinkID})
	if err := runner.Run(cfg, handler.Reg, p2); err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, s := range snaps {
		total += len(s.Rows)
	}
	if total < 2 {
		t.Fatalf("rows %d", total)
	}
	names := map[string]bool{}
	for _, s := range snaps {
		for i := range s.Rows {
			if n, ok := s.Cell(i, "name").(string); ok {
				names[n] = true
			}
		}
	}
	if !names["alice"] || !names["bob"] {
		t.Fatalf("rows %+v", snaps)
	}
}

func TestXLSXFixtureXLSB(t *testing.T) {
	swltest.RegisterHandlers()
	path := filepath.Join("..", "..", "testdata", "xlsx", "simple.xlsb")
	if _, err := os.Stat(path); err != nil {
		t.Skip("xlsb fixture missing")
	}
	p, err := pipeline.Parse([]string{path}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if p.Stages[0].ID != "xlsx-src" {
		t.Fatalf("stage %s", p.Stages[0].ID)
	}
	cfg := handlers.Config{Ctx: context.Background()}
	h, ok := handler.Get("xlsx-src")
	if !ok {
		t.Fatal("xlsx-src not registered")
	}
	src := h.(handlers.Source)
	s, err := src.Source(context.Background(), cfg, p.Stages[0].Options)
	if err != nil {
		t.Fatal(err)
	}
	snaps, err := swltest.CollectStream(t, s)
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 2 {
		t.Fatalf("snaps %+v", snaps)
	}
}

func TestXLSXFixtureODS(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "xlsx", "simple.ods")
	if _, err := os.Stat(path); err != nil {
		t.Skip("ods fixture missing")
	}
	p, err := pipeline.Parse([]string{path}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if p.Stages[0].ID != "xlsx-src" {
		t.Fatalf("stage %s", p.Stages[0].ID)
	}
	cfg := handlers.Config{Ctx: context.Background()}
	h, ok := handler.Get("xlsx-src")
	if !ok {
		t.Fatal("xlsx-src not registered")
	}
	src := h.(handlers.Source)
	s, err := src.Source(context.Background(), cfg, p.Stages[0].Options)
	if err != nil {
		t.Fatal(err)
	}
	snaps, err := swltest.CollectStream(t, s)
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) < 1 || len(snaps[0].Rows) != 2 {
		t.Fatalf("snaps %+v", snaps)
	}
}

func TestXLSXSinkRoundTripPipeline(t *testing.T) {
	swltest.RegisterHandlers()

	xlsxPath := filepath.Join("..", "..", "testdata", "xlsx", "simple.xlsx")
	outPath := filepath.Join(t.TempDir(), "out.xlsx")

	p, err := pipeline.Parse([]string{xlsxPath, "::", outPath}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Stages) != 2 || p.Stages[1].ID != "xlsx-sink" {
		t.Fatalf("stages %+v", p.Stages)
	}
	cfg := handlers.Config{Ctx: context.Background()}
	if err := runner.Run(cfg, handler.Reg, p); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatal(err)
	}
}
