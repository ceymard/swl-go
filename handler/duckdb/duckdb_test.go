package duckdb_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/ceymard/swl-go/handler/duckdb"
	"github.com/ceymard/swl-go/handler/json"
	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/handlers"
	"github.com/ceymard/swl-go/internal/msg"
	"github.com/ceymard/swl-go/test/swltest"

	_ "github.com/duckdb/duckdb-go/v2"
)

func createUsersDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.duckdb")
	db, err := sql.Open("duckdb", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE users (id INTEGER, name VARCHAR)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users VALUES (1, 'alice'), (2, 'bob')`); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSourceAllTables(t *testing.T) {
	path := createUsersDB(t)
	opts, err := duckdb.ParseSrcOptions(path, nil)
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
	if len(snaps) != 1 || snaps[0].Name != "users" || len(snaps[0].Rows) != 2 {
		t.Fatalf("got %+v", snaps)
	}
}

func TestSourceCustomQuery(t *testing.T) {
	path := createUsersDB(t)
	opts, err := duckdb.ParseSrcOptions(path, []string{"users", "-q", "SELECT id FROM users WHERE id = 1"})
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
	if snaps[0].Rows[0]["id"] == nil {
		t.Fatalf("row %+v", snaps[0].Rows[0])
	}
}

func TestSinkRoundTrip(t *testing.T) {
	path := createUsersDB(t)
	outPath := filepath.Join(t.TempDir(), "out.duckdb")

	srcOpts, _ := duckdb.ParseSrcOptions(path, nil)
	in, err := duckdb.Source{}.Source(context.Background(), handlers.Config{}, srcOpts)
	if err != nil {
		t.Fatal(err)
	}

	sinkOpts, _ := duckdb.ParseSinkOptions(outPath, nil)
	sink := duckdb.Sink{}
	if err := sink.Sink(context.Background(), handlers.Config{Messages: msg.New(0)}, in, sinkOpts); err != nil {
		t.Fatal(err)
	}

	outOpts, _ := duckdb.ParseSrcOptions(outPath, nil)
	out, err := duckdb.Source{}.Source(context.Background(), handlers.Config{}, outOpts)
	if err != nil {
		t.Fatal(err)
	}
	snaps, err := swltest.CollectStream(t, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps[0].Rows) != 2 {
		t.Fatalf("rows %d", len(snaps[0].Rows))
	}
}

func TestJSONToDuckDB(t *testing.T) {
	jsonPath := filepath.Join("..", "..", "testdata", "json", "simple.json")
	outPath := filepath.Join(t.TempDir(), "out.duckdb")

	jsonOpts, _ := json.ParseSrcOptions(jsonPath, nil)
	in, err := json.Source{}.Source(context.Background(), handlers.Config{}, jsonOpts)
	if err != nil {
		t.Fatal(err)
	}

	sinkOpts, _ := duckdb.ParseSinkOptions(outPath, nil)
	sink := duckdb.Sink{}
	if err := sink.Sink(context.Background(), handlers.Config{Messages: msg.New(0)}, in, sinkOpts); err != nil {
		t.Fatal(err)
	}

	outOpts, _ := duckdb.ParseSrcOptions(outPath, nil)
	out, err := duckdb.Source{}.Source(context.Background(), handlers.Config{}, outOpts)
	if err != nil {
		t.Fatal(err)
	}
	snaps, err := swltest.CollectStream(t, out)
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 1 || len(snaps[0].Rows) != 2 {
		t.Fatalf("got %+v", snaps)
	}
}

func TestSinkSchemaTable(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "out.duckdb")
	stream := func(yield func(coll.Collection, error) bool) {
		rows := func(yieldRow func(coll.Row, error) bool) {
			yieldRow(coll.Row{"id": int64(1), "name": "alice"}, nil)
		}
		yield(coll.Collection{Name: "analytics.users", Rows: rows}, nil)
	}

	sinkOpts, _ := duckdb.ParseSinkOptions(outPath, nil)
	sink := duckdb.Sink{}
	if err := sink.Sink(context.Background(), handlers.Config{Messages: msg.New(0)}, stream, sinkOpts); err != nil {
		t.Fatal(err)
	}

	opts, _ := duckdb.ParseSrcOptions(outPath, []string{"analytics.users"})
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
}
