package duckdb_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/ceymard/swl-go/handler/duckdb"
	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/handlers"
	"github.com/ceymard/swl-go/internal/msg"
	"github.com/ceymard/swl-go/test/swltest"

	_ "github.com/duckdb/duckdb-go/v2"
)

func createComplexTypesDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "complex.duckdb")
	db, err := sql.Open("duckdb", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE complex_types (
		id INTEGER,
		tags VARCHAR[],
		meta MAP(VARCHAR, VARCHAR),
		attrs STRUCT(k VARCHAR, v INTEGER)
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO complex_types VALUES (
		1,
		['alpha', 'beta'],
		map(['role', 'team'], ['admin', 'ops']),
		{'k': 'x', 'v': 42}
	)`); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSourceComplexTypes(t *testing.T) {
	path := createComplexTypesDB(t)
	opts, err := duckdb.ParseSrcOptions(path, []string{"complex_types"})
	if err != nil {
		t.Fatal(err)
	}
	s, err := duckdb.Source{}.Source(context.Background(), handlers.Config{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	snaps, err := swltest.CollectStream(t, s)
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 1 || len(snaps[0].Rows) != 1 {
		t.Fatalf("got %+v", snaps)
	}
	row := snaps[0].Rows[0]

	tags, ok := row["tags"].([]any)
	if !ok {
		t.Fatalf("tags type %T", row["tags"])
	}
	if !reflect.DeepEqual(tags, []any{"alpha", "beta"}) {
		t.Fatalf("tags %+v", tags)
	}

	meta, ok := row["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta type %T", row["meta"])
	}
	if meta["role"] != "admin" || meta["team"] != "ops" {
		t.Fatalf("meta %+v", meta)
	}

	attrs, ok := row["attrs"].(map[string]any)
	if !ok {
		t.Fatalf("attrs type %T", row["attrs"])
	}
	if attrs["k"] != "x" {
		t.Fatalf("attrs k %+v", attrs["k"])
	}
	switch v := attrs["v"].(type) {
	case int, int32, int64:
		if reflect.ValueOf(v).Int() != 42 {
			t.Fatalf("attrs v %v", v)
		}
	case float64:
		if v != 42 {
			t.Fatalf("attrs v %v", v)
		}
	default:
		t.Fatalf("attrs v type %T", v)
	}
}

func TestSinkComplexTypesRoundTrip(t *testing.T) {
	path := createComplexTypesDB(t)
	outPath := filepath.Join(t.TempDir(), "out.duckdb")

	srcOpts, _ := duckdb.ParseSrcOptions(path, []string{"complex_types"})
	in, err := duckdb.Source{}.Source(context.Background(), handlers.Config{}, srcOpts)
	if err != nil {
		t.Fatal(err)
	}

	sinkOpts, _ := duckdb.ParseSinkOptions(outPath, nil)
	sink := duckdb.Sink{}
	if err := sink.Sink(context.Background(), handlers.Config{Messages: msg.New(0)}, in, sinkOpts); err != nil {
		t.Fatal(err)
	}

	outOpts, _ := duckdb.ParseSrcOptions(outPath, []string{"complex_types"})
	out, err := duckdb.Source{}.Source(context.Background(), handlers.Config{}, outOpts)
	if err != nil {
		t.Fatal(err)
	}
	snaps, err := swltest.CollectStream(t, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 1 || len(snaps[0].Rows) != 1 {
		t.Fatalf("got %+v", snaps)
	}

	row := snaps[0].Rows[0]
	tags, ok := row["tags"].([]any)
	if !ok || !reflect.DeepEqual(tags, []any{"alpha", "beta"}) {
		t.Fatalf("tags %+v", row["tags"])
	}
	meta, ok := row["meta"].(map[string]any)
	if !ok || meta["role"] != "admin" {
		t.Fatalf("meta %+v", row["meta"])
	}
}

func TestSourceJSONColumnWithNestedData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "json.duckdb")
	db, err := sql.Open("duckdb", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE docs (id INTEGER, payload JSON)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO docs VALUES (1, '{"users":[{"id":1,"tags":["a","b"]}],"meta":{"count":2,"nested":{"ok":true}}}')`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	opts, _ := duckdb.ParseSrcOptions(path, []string{"docs"})
	s, err := duckdb.Source{}.Source(context.Background(), handlers.Config{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	snaps, err := swltest.CollectStream(t, s)
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := snaps[0].Rows[0]["payload"].(map[string]any)
	if !ok {
		t.Fatalf("payload type %T", snaps[0].Rows[0]["payload"])
	}
	meta, ok := payload["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta type %T", payload["meta"])
	}
	nested, ok := meta["nested"].(map[string]any)
	if !ok || nested["ok"] != true {
		t.Fatalf("nested %+v", meta["nested"])
	}
	users, ok := payload["users"].([]any)
	if !ok || len(users) != 1 {
		t.Fatalf("users %+v", payload["users"])
	}
	user0, ok := users[0].(map[string]any)
	if !ok {
		t.Fatalf("user0 type %T", users[0])
	}
	userTags, ok := user0["tags"].([]any)
	if !ok || len(userTags) != 2 {
		t.Fatalf("user tags %+v", user0["tags"])
	}
}

func TestSinkJSONColumnRoundTrip(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "out.duckdb")
	stream := func(yield func(coll.Collection, error) bool) {
		rows := func(yieldBatch func([]coll.Row, error) bool) {
			yieldBatch([]coll.Row{{
				"id": int64(1),
				"payload": map[string]any{
					"users": []any{
						map[string]any{"id": int64(1), "tags": []any{"a", "b"}},
					},
					"meta": map[string]any{
						"count": int64(2),
						"nested": map[string]any{"ok": true},
					},
				},
			}}, nil)
		}
		yield(coll.Collection{Name: "docs", Rows: rows}, nil)
	}

	sinkOpts, _ := duckdb.ParseSinkOptions(outPath, nil)
	sink := duckdb.Sink{}
	if err := sink.Sink(context.Background(), handlers.Config{Messages: msg.New(0)}, stream, sinkOpts); err != nil {
		t.Fatal(err)
	}

	opts, _ := duckdb.ParseSrcOptions(outPath, []string{"docs"})
	s, err := duckdb.Source{}.Source(context.Background(), handlers.Config{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	snaps, err := swltest.CollectStream(t, s)
	if err != nil {
		t.Fatal(err)
	}
	payload := snaps[0].Rows[0]["payload"].(map[string]any)
	meta := payload["meta"].(map[string]any)
	switch c := meta["count"].(type) {
	case int64:
		if c != 2 {
			t.Fatalf("count %v", c)
		}
	case int32:
		if c != 2 {
			t.Fatalf("count %v", c)
		}
	case float64:
		if c != 2 {
			t.Fatalf("count %v", c)
		}
	default:
		t.Fatalf("count type %T", meta["count"])
	}
}
