package json_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ceymard/swl-go/handler/json"
	"github.com/ceymard/swl-go/internal/handlers"
	"github.com/ceymard/swl-go/internal/msg"
	"github.com/ceymard/swl-go/test/swltest"
)

func TestSourceSimpleArray(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "json", "simple.json")
	opts, err := json.ParseSrcOptions(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	cfg := handlers.Config{Ctx: context.Background(), Messages: msg.New(0)}
	stream, err := json.Source{}.Source(cfg.Ctx, cfg, opts)
	if err != nil {
		t.Fatal(err)
	}
	snaps, err := swltest.CollectStream(t, stream)
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 1 {
		t.Fatalf("collections %d", len(snaps))
	}
	if snaps[0].Name != "simple" {
		t.Fatalf("name %q", snaps[0].Name)
	}
	if len(snaps[0].Rows) != 2 {
		t.Fatalf("rows %d", len(snaps[0].Rows))
	}
}

func TestSourceInlineObject(t *testing.T) {
	opts, err := json.ParseSrcOptions(`{"a":1,"b":2}`, nil)
	if err != nil {
		t.Fatal(err)
	}
	cfg := handlers.Config{Ctx: context.Background()}
	stream, err := json.Source{}.Source(cfg.Ctx, cfg, opts)
	if err != nil {
		t.Fatal(err)
	}
	snaps, err := swltest.CollectStream(t, stream)
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 1 || snaps[0].Name != "json" {
		t.Fatalf("got %+v", snaps)
	}
	if len(snaps[0].Rows) != 1 {
		t.Fatalf("rows %d", len(snaps[0].Rows))
	}
	if snaps[0].Rows[0]["a"] == nil {
		t.Fatalf("row %+v", snaps[0].Rows[0])
	}
}

func TestSourceMultiCollectionFile(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "json", "multi.json")
	opts, err := json.ParseSrcOptions(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := json.Source{}.Source(context.Background(), handlers.Config{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	snaps, err := swltest.CollectStream(t, stream)
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 2 {
		t.Fatalf("collections %d", len(snaps))
	}
	names := map[string]int{}
	for _, s := range snaps {
		names[s.Name] = len(s.Rows)
	}
	if names["users"] != 1 || names["orders"] != 1 {
		t.Fatalf("names %+v", names)
	}
}

func TestSinkRoundTrip(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join("..", "..", "testdata", "json", "simple.json")
	outPath := filepath.Join(dir, "out.json")

	srcOpts, _ := json.ParseSrcOptions(inPath, nil)
	stream, err := json.Source{}.Source(context.Background(), handlers.Config{}, srcOpts)
	if err != nil {
		t.Fatal(err)
	}

	sinkOpts, _ := json.ParseSinkOptions(outPath, nil)
	sink := json.Sink{}
	if err := sink.Sink(context.Background(), handlers.Config{}, stream, sinkOpts); err != nil {
		t.Fatal(err)
	}

	b, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) == 0 {
		t.Fatal("empty output")
	}
}

func TestSinkDirectory(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join("..", "..", "testdata", "json", "multi.json")
	outDir := filepath.Join(dir, "out")
	if err := os.Mkdir(outDir, 0o755); err != nil {
		t.Fatal(err)
	}

	srcOpts, _ := json.ParseSrcOptions(inPath, nil)
	stream, err := json.Source{}.Source(context.Background(), handlers.Config{}, srcOpts)
	if err != nil {
		t.Fatal(err)
	}
	sinkOpts, _ := json.ParseSinkOptions(outDir, nil)
	sink := json.Sink{}
	if err := sink.Sink(context.Background(), handlers.Config{}, stream, sinkOpts); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"users.json", "orders.json"} {
		if _, err := os.Stat(filepath.Join(outDir, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
}
