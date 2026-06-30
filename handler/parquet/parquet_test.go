package parquet_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/ceymard/swl-go/handler/parquet"
	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/handlers"
	"github.com/ceymard/swl-go/internal/pipeline"
)

func writeFixture(t *testing.T, path string, rows []map[string]any) {
	t.Helper()
	if err := parquet.WriteParquetFileForTest(path, rows); err != nil {
		t.Fatal(err)
	}
}

func TestParseSrcOptionsMultipleFiles(t *testing.T) {
	opts, err := parquet.ParseSrcOptions("a-0000001.pqt", []string{"b-0000002.pqt", "-c", "id,name"})
	if err != nil {
		t.Fatal(err)
	}
	o := opts.(parquet.SrcOpts)
	if len(o.Selections) != 2 {
		t.Fatalf("selections %d", len(o.Selections))
	}
	if o.Selections[1].Columns != "id,name" {
		t.Fatalf("columns %q", o.Selections[1].Columns)
	}
}

func TestSourceMergeArchiveShards(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, filepath.Join(dir, "orders-0000001.parquet"), []map[string]any{
		{"id": int64(1), "name": "alice"},
	})
	writeFixture(t, filepath.Join(dir, "orders-0000002.parquet"), []map[string]any{
		{"id": int64(2), "name": "bob"},
	})

	opts, _ := parquet.ParseSrcOptions(
		filepath.Join(dir, "orders-0000001.parquet"),
		[]string{filepath.Join(dir, "orders-0000002.parquet")},
	)
	stream, err := parquet.Source{}.Source(context.Background(), handlers.Config{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	var snaps []collRowSnap
	for c, err := range stream {
		if err != nil {
			t.Fatal(err)
		}
		snap := collRowSnap{Name: c.Name}
		for row, err := range c.Rows {
			if err != nil {
				t.Fatal(err)
			}
			snap.Rows = append(snap.Rows, row)
		}
		snaps = append(snaps, snap)
	}
	if len(snaps) != 1 || snaps[0].Name != "orders" || len(snaps[0].Rows) != 2 {
		t.Fatalf("snaps %+v", snaps)
	}
}

func TestSourceColumnProjection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "people.parquet")
	writeFixture(t, path, []map[string]any{
		{"id": int64(1), "name": "alice", "extra": "x"},
	})

	opts, _ := parquet.ParseSrcOptions(path, []string{"-c", "id,name"})
	stream, err := parquet.Source{}.Source(context.Background(), handlers.Config{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	for c, err := range stream {
		if err != nil {
			t.Fatal(err)
		}
		for row, err := range c.Rows {
			if err != nil {
				t.Fatal(err)
			}
			if _, ok := row["extra"]; ok {
				t.Fatalf("extra column present: %+v", row)
			}
			if row["name"] != "alice" {
				t.Fatalf("row %+v", row)
			}
		}
	}
}

func TestSinkWriteAndReadBack(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "people.parquet")

	stream := func(yield func(coll.Collection, error) bool) {
		rows := coll.SliceRows([]coll.Row{
			{"id": int64(1), "name": "alice"},
			{"id": int64(2), "name": "bob"},
		})
		yield(coll.Collection{Name: "people", Rows: rows}, nil)
	}
	sink := parquet.Sink{}
	opts, _ := parquet.ParseSinkOptions(out, nil)
	if err := sink.Sink(context.Background(), handlers.Config{}, stream, opts); err != nil {
		t.Fatal(err)
	}

	readOpts, _ := parquet.ParseSrcOptions(out, nil)
	readStream, err := parquet.Source{}.Source(context.Background(), handlers.Config{}, readOpts)
	if err != nil {
		t.Fatal(err)
	}
	for c, err := range readStream {
		if err != nil {
			t.Fatal(err)
		}
		n := 0
		for _, err := range c.Rows {
			if err != nil {
				t.Fatal(err)
			}
			n++
		}
		if n != 2 {
			t.Fatalf("rows %d", n)
		}
	}
}

func TestSinkDirectoryLayout(t *testing.T) {
	dir := t.TempDir()
	outDir := filepath.Join(dir, "out")
	stream := func(yield func(coll.Collection, error) bool) {
		yield(coll.Collection{Name: "users", Rows: coll.SliceRows([]coll.Row{{"id": int64(1)}})}, nil)
	}
	sink := parquet.Sink{}
	opts, _ := parquet.ParseSinkOptions(outDir, nil)
	if err := sink.Sink(context.Background(), handlers.Config{}, stream, opts); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "users.parquet")); err != nil {
		t.Fatal(err)
	}
}

func TestPipelineParseParquetSink(t *testing.T) {
	p, err := pipeline.Parse([]string{"data.json", "::", "out.parquet"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if p.Stages[1].ID != "parquet-sink" {
		t.Fatalf("stage %s", p.Stages[1].ID)
	}
}

type collRowSnap struct {
	Name string
	Rows []coll.Row
}