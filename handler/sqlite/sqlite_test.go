package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/ceymard/swl-go/handler/json"
	"github.com/ceymard/swl-go/handler/sqlite"
	"github.com/ceymard/swl-go/internal/handlers"
	"github.com/ceymard/swl-go/internal/msg"
	"github.com/ceymard/swl-go/test/swltest"

	_ "github.com/mattn/go-sqlite3"
)

func createUsersDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO users (id, name) VALUES (1, 'alice'), (2, 'bob')`); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestSourceAllTables(t *testing.T) {
	path := createUsersDB(t)
	opts, err := sqlite.ParseSrcOptions(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	s, err := sqlite.Source{}.Source(context.Background(), handlers.Config{}, opts)
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

func TestSinkRoundTrip(t *testing.T) {
	path := createUsersDB(t)
	outPath := filepath.Join(t.TempDir(), "out.db")

	srcOpts, _ := sqlite.ParseSrcOptions(path, nil)
	in, err := sqlite.Source{}.Source(context.Background(), handlers.Config{}, srcOpts)
	if err != nil {
		t.Fatal(err)
	}

	sinkOpts, _ := sqlite.ParseSinkOptions(outPath, nil)
	sink := sqlite.Sink{}
	if err := sink.Sink(context.Background(), handlers.Config{Messages: msg.New(0)}, in, sinkOpts); err != nil {
		t.Fatal(err)
	}

	outOpts, _ := sqlite.ParseSrcOptions(outPath, nil)
	out, err := sqlite.Source{}.Source(context.Background(), handlers.Config{}, outOpts)
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

func TestSourceDeclaredJSONColumns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "arrays.db")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE items (id INTEGER PRIMARY KEY, tags text[], note TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO items (id, tags, note) VALUES (1, '["alpha","beta"]', '["ignored"]')`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	opts, err := sqlite.ParseSrcOptions(path, []string{"items"})
	if err != nil {
		t.Fatal(err)
	}
	s, err := sqlite.Source{}.Source(context.Background(), handlers.Config{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	snaps, err := swltest.CollectStream(t, s)
	if err != nil {
		t.Fatal(err)
	}
	tags, ok := snaps[0].Cell(0, "tags").([]any)
	if !ok {
		t.Fatalf("tags type %T value %#v", snaps[0].Cell(0, "tags"), snaps[0].Cell(0, "tags"))
	}
	if len(tags) != 2 || tags[0] != "alpha" || tags[1] != "beta" {
		t.Fatalf("tags %#v", tags)
	}
	if note, ok := snaps[0].Cell(0, "note").(string); !ok || note != `["ignored"]` {
		t.Fatalf("note %#v", snaps[0].Cell(0, "note"))
	}
}

func TestJSONToSQLite(t *testing.T) {
	jsonPath := filepath.Join("..", "..", "testdata", "json", "simple.json")
	outPath := filepath.Join(t.TempDir(), "out.db")

	jsonOpts, _ := json.ParseSrcOptions(jsonPath, nil)
	in, err := json.Source{}.Source(context.Background(), handlers.Config{}, jsonOpts)
	if err != nil {
		t.Fatal(err)
	}

	sinkOpts, _ := sqlite.ParseSinkOptions(outPath, nil)
	sink := sqlite.Sink{}
	if err := sink.Sink(context.Background(), handlers.Config{Messages: msg.New(0)}, in, sinkOpts); err != nil {
		t.Fatal(err)
	}

	outOpts, _ := sqlite.ParseSrcOptions(outPath, nil)
	out, err := sqlite.Source{}.Source(context.Background(), handlers.Config{}, outOpts)
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
