package yaml_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ceymard/swl-go/handler/yaml"
	"github.com/ceymard/swl-go/internal/handlers"
	"github.com/ceymard/swl-go/test/swltest"
)

func TestSourceSimpleMapping(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "yaml", "simple.yml")
	opts, err := yaml.ParseSrcOptions(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := yaml.Source{}.Source(context.Background(), handlers.Config{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	snaps, err := swltest.CollectStream(t, stream)
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 1 || snaps[0].Name != "users" || len(snaps[0].Rows) != 2 {
		t.Fatalf("got %+v", snaps)
	}
	if snaps[0].Cell(0, "name") != "alice" {
		t.Fatalf("row %+v", snaps[0].Rows[0])
	}
}

func TestSourceTopLevelArray(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "yaml", "array.yml")
	opts, err := yaml.ParseSrcOptions(path, []string{"-c", "people"})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := yaml.Source{}.Source(context.Background(), handlers.Config{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	snaps, err := swltest.CollectStream(t, stream)
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 1 || snaps[0].Name != "people" || len(snaps[0].Rows) != 2 {
		t.Fatalf("got %+v", snaps)
	}
}

func TestSourceJSGenerators(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "yaml", "generators.yml")
	opts, err := yaml.ParseSrcOptions(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := yaml.Source{}.Source(context.Background(), handlers.Config{}, opts)
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
	byName := map[string]swltest.Snapshot{}
	for _, s := range snaps {
		byName[s.Name] = s
	}
	static := byName["static"]
	if len(static.Rows) != 1 || static.Cell(0, "id") != int64(1) {
		t.Fatalf("static %+v", static.Rows)
	}
	gen := byName["generated"]
	if len(gen.Rows) != 2 {
		t.Fatalf("generated rows %d", len(gen.Rows))
	}
	if gen.Cell(0, "ref") != "base" {
		t.Fatalf("generated[0] %+v", gen.Rows[0])
	}
	if gen.HasColumn("__meta__") {
		t.Fatalf("expected __meta__ stripped: %+v", gen.Rows[1])
	}
}

func TestSinkRoundTrip(t *testing.T) {
	dir := t.TempDir()
	inPath := filepath.Join("..", "..", "testdata", "yaml", "simple.yml")
	outPath := filepath.Join(dir, "out.yml")

	srcOpts, _ := yaml.ParseSrcOptions(inPath, nil)
	stream, err := yaml.Source{}.Source(context.Background(), handlers.Config{}, srcOpts)
	if err != nil {
		t.Fatal(err)
	}

	sinkOpts, _ := yaml.ParseSinkOptions(outPath, nil)
	sink := yaml.Sink{}
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
	inPath := filepath.Join("..", "..", "testdata", "yaml", "generators.yml")
	outDir := filepath.Join(dir, "out")
	if err := os.Mkdir(outDir, 0o755); err != nil {
		t.Fatal(err)
	}

	srcOpts, _ := yaml.ParseSrcOptions(inPath, nil)
	stream, err := yaml.Source{}.Source(context.Background(), handlers.Config{}, srcOpts)
	if err != nil {
		t.Fatal(err)
	}
	sinkOpts, _ := yaml.ParseSinkOptions(outDir, nil)
	sink := yaml.Sink{}
	if err := sink.Sink(context.Background(), handlers.Config{}, stream, sinkOpts); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"static.yml", "generated.yml"} {
		if _, err := os.Stat(filepath.Join(outDir, name)); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}
}

func TestParseOptions(t *testing.T) {
	src, err := yaml.ParseSrcOptions("data.yml", []string{"-c", "items", "-e", "utf-8"})
	if err != nil {
		t.Fatal(err)
	}
	o := src.(yaml.SrcOpts)
	if o.File != "data.yml" || o.Collection == nil || *o.Collection != "items" {
		t.Fatalf("%+v", o)
	}

	sink, err := yaml.ParseSinkOptions("out/%", nil)
	if err != nil {
		t.Fatal(err)
	}
	if sink.(yaml.SinkOpts).Path != "out/%" {
		t.Fatalf("%+v", sink)
	}
}
