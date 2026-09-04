package swlbench_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	csvh "github.com/ceymard/swl-go/handler/csv"
	"github.com/ceymard/swl-go/handler/flatten"
	jsonh "github.com/ceymard/swl-go/handler/json"
	"github.com/ceymard/swl-go/handler/sqlite"
	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/handlers"
	"github.com/ceymard/swl-go/internal/jsonx"
	"github.com/ceymard/swl-go/internal/progress"
	"github.com/ceymard/swl-go/internal/stream"
)

var (
	benchRows []coll.Row
	benchCols *coll.ColumnSet
)

func init() {
	path := filepath.Join("..", "..", "testdata", "json", "bench50k.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var parsed map[string][]map[string]any
	if err := jsonx.Unmarshal(b, &parsed); err != nil {
		return
	}
	benchCols = coll.NewColumnSet()
	for _, m := range parsed["users"] {
		benchRows = append(benchRows, coll.RowFromMap(benchCols, m))
	}
}

func BenchmarkJSONParse50k(b *testing.B) {
	path := filepath.Join("..", "..", "testdata", "json", "bench50k.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		b.Skip(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var parsed any
		if err := jsonx.Unmarshal(raw, &parsed); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJSONMarshalRow(b *testing.B) {
	if len(benchRows) == 0 {
		b.Skip("bench fixture missing")
	}
	row := benchRows[0]
	cols := benchCols.Columns()
	m := make(map[string]any, len(cols))
	for i, c := range cols {
		m[c.ColumnName] = row.Cell(i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := jsonx.Marshal(m); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkFlattenRow(b *testing.B) {
	if len(benchRows) == 0 {
		b.Skip("bench fixture missing")
	}
	inCols := coll.NewColumnSet()
	row := coll.RowFromMap(inCols, map[string]any{
		"user": map[string]any{
			"name": "Ann",
			"tags": []any{"a", "b", "c"},
			"meta": map[string]any{"score": 1.5, "active": true},
		},
	})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		outCols := coll.NewColumnSet()
		_ = flatten.Flatten(inCols, row, outCols)
	}
}

func BenchmarkSQLiteInsert50k(b *testing.B) {
	if len(benchRows) == 0 {
		b.Skip("bench fixture missing")
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		path := filepath.Join(b.TempDir(), "bench.db")
		b.StartTimer()

		s := stream.Of(coll.Collection{Name: "users", Columns: benchCols, Rows: coll.SliceRowBatches(benchRows)})
		opts, _ := sqlite.ParseSinkOptions(path, nil)
		if err := (sqlite.Sink{}).Sink(context.Background(), handlers.Config{Verbose: 0}, s, opts); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkJSONSource50k(b *testing.B) {
	path := filepath.Join("..", "..", "testdata", "json", "bench50k.json")
	opts, err := jsonh.ParseSrcOptions(path, nil)
	if err != nil {
		b.Fatal(err)
	}
	cfg := handlers.Config{Ctx: context.Background(), Verbose: 0}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		stream, err := jsonh.Source{}.Source(cfg.Ctx, cfg, opts)
		if err != nil {
			b.Fatal(err)
		}
		n := 0
		for c, err := range stream {
			if err != nil {
				b.Fatal(err)
			}
			for batch, err := range c.Rows {
				if err != nil {
					b.Fatal(err)
				}
				n += len(batch)
			}
		}
		if n != len(benchRows) {
			b.Fatalf("rows %d", n)
		}
	}
}

// BenchmarkPipelineJSONFlattenSQLite50k mirrors runner.Run's stage wiring
// (source -> CheckContext -> progress.Track -> transform -> CheckContext ->
// progress.Track -> sink) so it exercises the per-row plumbing overhead
// (context checks, progress counters) that batching targets, unlike
// BenchmarkJSONSource50k which reads the raw source stream directly.
func BenchmarkPipelineJSONFlattenSQLite50k(b *testing.B) {
	if len(benchRows) == 0 {
		b.Skip("bench fixture missing")
	}
	path := filepath.Join("..", "..", "testdata", "json", "bench50k.json")
	srcOpts, err := jsonh.ParseSrcOptions(path, nil)
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		dbPath := filepath.Join(b.TempDir(), "bench.db")
		sinkOpts, _ := sqlite.ParseSinkOptions(dbPath, nil)
		b.StartTimer()

		cfg := handlers.Config{Ctx: ctx, Verbose: 0}
		s, err := jsonh.Source{}.Source(ctx, cfg, srcOpts)
		if err != nil {
			b.Fatal(err)
		}
		s = progress.Track(cfg.Messages, progress.Source, stream.CheckContext(ctx, s))

		s, err = (flatten.Transform{}).Transform(ctx, cfg, s, nil)
		if err != nil {
			b.Fatal(err)
		}
		s = stream.CheckContext(ctx, s)

		s = progress.Track(cfg.Messages, progress.Sink, s)
		if err := (sqlite.Sink{}).Sink(ctx, cfg, s, sinkOpts); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkPipelineCSVSQLite50k mirrors BenchmarkPipelineJSONFlattenSQLite50k
// but for a fixed-schema tabular source/sink with no transform in between —
// the "mostly tabular" workload the ColumnSet refactor's hypothesis is
// actually about (JSON+flatten retains per-key ColumnSet.Index hashing by
// design; this path shouldn't).
func BenchmarkPipelineCSVSQLite50k(b *testing.B) {
	path := filepath.Join("..", "..", "testdata", "csv", "bench50k.csv")
	if _, err := os.Stat(path); err != nil {
		b.Skip("csv bench fixture missing")
	}
	srcOpts, err := csvh.ParseSrcOptions(path, nil)
	if err != nil {
		b.Fatal(err)
	}
	ctx := context.Background()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		b.StopTimer()
		dbPath := filepath.Join(b.TempDir(), "bench.db")
		sinkOpts, _ := sqlite.ParseSinkOptions(dbPath, nil)
		b.StartTimer()

		cfg := handlers.Config{Ctx: ctx, Verbose: 0}
		s, err := csvh.Source{}.Source(ctx, cfg, srcOpts)
		if err != nil {
			b.Fatal(err)
		}
		s = progress.Track(cfg.Messages, progress.Source, stream.CheckContext(ctx, s))
		s = progress.Track(cfg.Messages, progress.Sink, s)
		if err := (sqlite.Sink{}).Sink(ctx, cfg, s, sinkOpts); err != nil {
			b.Fatal(err)
		}
	}
}
