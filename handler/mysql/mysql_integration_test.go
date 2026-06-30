package mysql_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"

	"github.com/ceymard/swl-go/handler/mysql"
	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/handlers"
	"github.com/ceymard/swl-go/internal/msg"
	"github.com/ceymard/swl-go/test/swltest"
	"github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
	"github.com/testcontainers/testcontainers-go/wait"

	_ "github.com/go-sql-driver/mysql"
)

var (
	mysqlContainer testcontainers.Container
	mysqlURI       string
)

func TestMain(m *testing.M) {
	code := 1
	defer func() { os.Exit(code) }()
	if os.Getenv("SKIP_TESTCONTAINERS") == "" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		container, err := tcmysql.Run(ctx, "mysql:8.4",
			tcmysql.WithDatabase("swltest"),
			tcmysql.WithUsername("swl"),
			tcmysql.WithPassword("swl"),
			testcontainers.WithWaitStrategy(
				wait.ForLog("port: 3306  MySQL Community Server").WithStartupTimeout(90*time.Second),
			),
		)
		cancel()
		if err == nil {
			mysqlContainer = container
			ctx2, cancel2 := context.WithTimeout(context.Background(), time.Minute)
			mysqlURI, _ = container.ConnectionString(ctx2, "parseTime=true", "charset=utf8mb4")
			cancel2()
		}
	}
	code = m.Run()
	if mysqlContainer != nil {
		_ = testcontainers.TerminateContainer(mysqlContainer)
	}
}

func handlersConfig() handlers.Config {
	return handlers.Config{Messages: msg.New(0)}
}

func startMySQL(t *testing.T) string {
	t.Helper()
	if mysqlURI == "" {
		t.Skip("mysql testcontainer unavailable (docker required, or set SKIP_TESTCONTAINERS=1 to skip)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	applyComplexFixtures(t, ctx, mysqlURI)
	return mysqlURI
}

func applyComplexFixtures(t *testing.T, ctx context.Context, connStr string) {
	t.Helper()
	sqlBytes, err := os.ReadFile(complexFixturePath())
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("mysql", connStr+"&multiStatements=true")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, string(sqlBytes)); err != nil {
		t.Fatalf("apply complex fixtures: %v", err)
	}
}

func complexFixturePath() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "mysql", "complex_types.sql")
}

func execSQL(t *testing.T, ctx context.Context, connStr, stmt string) {
	t.Helper()
	db, err := sql.Open("mysql", connStr)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		t.Fatalf("exec sql: %v", err)
	}
}

func collectSource(t *testing.T, uri string, tail ...string) []swltest.Snapshot {
	t.Helper()
	opts, err := mysql.ParseSrcOptions(uri, tail)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := mysql.Source{}.Source(context.Background(), handlersConfig(), opts)
	if err != nil {
		t.Fatal(err)
	}
	snaps, err := swltest.CollectStream(t, stream)
	if err != nil {
		t.Fatal(err)
	}
	return snaps
}

func TestIntegrationSourceComplexTypes(t *testing.T) {
	uri := startMySQL(t)
	snaps := collectSource(t, uri, "documents")
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

	payload, ok := row["payload"].(map[string]any)
	if !ok {
		t.Fatalf("payload type %T", row["payload"])
	}
	users, ok := payload["users"].([]any)
	if !ok || len(users) != 1 {
		t.Fatalf("users %+v", payload["users"])
	}
	user0, ok := users[0].(map[string]any)
	if !ok {
		t.Fatalf("user0 type %T", users[0])
	}
	profile, ok := user0["profile"].(map[string]any)
	if !ok || profile["active"] != true {
		t.Fatalf("profile %+v", user0["profile"])
	}

	meta, ok := payload["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta type %T", payload["meta"])
	}
	nested, ok := meta["nested"].(map[string]any)
	if !ok {
		t.Fatalf("nested type %T", meta["nested"])
	}
	labels, ok := nested["labels"].([]any)
	if !ok || !reflect.DeepEqual(labels, []any{"x", "y"}) {
		t.Fatalf("labels %+v", nested["labels"])
	}
}

func TestIntegrationSinkComplexTypesRoundTrip(t *testing.T) {
	uri := startMySQL(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	execSQL(t, ctx, uri, "DROP TABLE IF EXISTS documents_copy")
	execSQL(t, ctx, uri, "CREATE TABLE documents_copy (id INT AUTO_INCREMENT PRIMARY KEY, tags JSON NOT NULL, payload JSON NOT NULL)")

	srcSnaps := collectSource(t, uri, "documents")
	in := renameCollectionStream(snapshotsToStream(srcSnaps), "documents", "documents_copy")

	sinkOpts, err := mysql.ParseSinkOptions(uri, nil)
	if err != nil {
		t.Fatal(err)
	}
	sink := mysql.Sink{}
	if err := sink.Sink(ctx, handlersConfig(), in, sinkOpts); err != nil {
		t.Fatal(err)
	}

	mirrorSnaps := collectSource(t, uri, "documents_copy")
	if len(mirrorSnaps) != 1 || len(mirrorSnaps[0].Rows) != 1 {
		t.Fatalf("mirror %+v", mirrorSnaps)
	}

	want := srcSnaps[0].Rows[0]
	got := mirrorSnaps[0].Rows[0]
	if !reflect.DeepEqual(got["tags"], want["tags"]) {
		t.Fatalf("tags mirror=%#v src=%#v", got["tags"], want["tags"])
	}
	if !reflect.DeepEqual(got["payload"], want["payload"]) {
		t.Fatalf("payload mirror=%#v src=%#v", got["payload"], want["payload"])
	}
}

func TestIntegrationSinkAutoCreateJSON(t *testing.T) {
	uri := startMySQL(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	stream := func(yield func(coll.Collection, error) bool) {
		rows := func(yieldRow func(coll.Row, error) bool) {
			yieldRow(coll.Row{
				"id":   int64(1),
				"tags": []any{"alpha", "beta"},
				"payload": map[string]any{
					"meta": map[string]any{"nested": map[string]any{"ok": true}},
				},
			}, nil)
		}
		yield(coll.Collection{Name: "created_docs", Rows: rows}, nil)
	}

	sinkOpts, err := mysql.ParseSinkOptions(uri, []string{"--auto-create"})
	if err != nil {
		t.Fatal(err)
	}
	sink := mysql.Sink{}
	if err := sink.Sink(ctx, handlersConfig(), stream, sinkOpts); err != nil {
		t.Fatal(err)
	}

	snaps := collectSource(t, uri, "created_docs")
	if len(snaps) != 1 || len(snaps[0].Rows) != 1 {
		t.Fatalf("got %+v", snaps)
	}
	payload := snaps[0].Rows[0]["payload"].(map[string]any)
	meta := payload["meta"].(map[string]any)
	if meta["nested"].(map[string]any)["ok"] != true {
		t.Fatalf("payload %+v", payload)
	}
}

func snapshotsToStream(snaps []swltest.Snapshot) coll.Stream {
	return func(yield func(coll.Collection, error) bool) {
		for _, s := range snaps {
			rows := append([]coll.Row(nil), s.Rows...)
			c := coll.Collection{
				Name: s.Name,
				Rows: coll.SliceRows(rows),
			}
			if !yield(c, nil) {
				return
			}
		}
	}
}

func renameCollectionStream(in coll.Stream, from, to string) coll.Stream {
	return func(yield func(coll.Collection, error) bool) {
		for c, err := range in {
			if err != nil {
				yield(coll.Collection{}, err)
				return
			}
			if c.Name == from {
				c.Name = to
			}
			if !yield(c, nil) {
				return
			}
		}
	}
}
