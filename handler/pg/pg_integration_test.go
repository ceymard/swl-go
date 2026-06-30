package pg_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ceymard/swl-go/handler/pg"
	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/handlers"
	"github.com/ceymard/swl-go/internal/msg"
	"github.com/ceymard/swl-go/test/swltest"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	pgContainer testcontainers.Container
	pgURI       string
)

func TestMain(m *testing.M) {
	code := 1
	defer func() { os.Exit(code) }()
	if os.Getenv("SKIP_TESTCONTAINERS") == "" {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		container, err := tcpostgres.Run(ctx,
			"postgres:16-alpine",
			tcpostgres.WithDatabase("swltest"),
			tcpostgres.WithUsername("swl"),
			tcpostgres.WithPassword("swl"),
			testcontainers.WithWaitStrategy(
				wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2).
					WithStartupTimeout(60*time.Second),
			),
		)
		cancel()
		if err == nil {
			pgContainer = container
			ctx2, cancel2 := context.WithTimeout(context.Background(), time.Minute)
			pgURI, _ = container.ConnectionString(ctx2, "sslmode=disable")
			cancel2()
		}
	}
	code = m.Run()
	if pgContainer != nil {
		_ = testcontainers.TerminateContainer(pgContainer)
	}
}

func handlersConfig() handlers.Config {
	return handlers.Config{Messages: msg.New(0)}
}

func startPostgres(t *testing.T) string {
	t.Helper()
	if pgURI == "" {
		t.Skip("postgres testcontainer unavailable (docker required, or set SKIP_TESTCONTAINERS=1 to skip)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	applyFixtures(t, ctx, pgURI)
	return pgURI
}

func applyFixtures(t *testing.T, ctx context.Context, connStr string) {
	t.Helper()
	sqlBytes, err := os.ReadFile(fixturePath())
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
		t.Fatalf("apply fixtures: %v", err)
	}
}

func fixturePath() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "pg", "fixtures.sql")
}

func complexTypesFixturePath() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "pg", "complex_types.sql")
}

func applyComplexFixtures(t *testing.T, ctx context.Context, connStr string) {
	t.Helper()
	sqlBytes, err := os.ReadFile(complexTypesFixturePath())
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, string(sqlBytes)); err != nil {
		t.Fatalf("apply complex fixtures: %v", err)
	}
}

func execSQL(t *testing.T, ctx context.Context, connStr, sql string) {
	t.Helper()
	pool, err := pgxpool.New(ctx, connStr)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, sql); err != nil {
		t.Fatalf("exec sql: %v", err)
	}
}

func collectSource(t *testing.T, uri string, tail ...string) []swltest.Snapshot {
	t.Helper()
	opts, err := pg.ParseSrcOptions(uri, tail)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := pg.Source{}.Source(context.Background(), handlersConfig(), opts)
	if err != nil {
		t.Fatal(err)
	}
	snaps, err := swltest.CollectStream(t, stream)
	if err != nil {
		t.Fatal(err)
	}
	return snaps
}

func collectionNames(snaps []swltest.Snapshot) []string {
	names := make([]string, len(snaps))
	for i, s := range snaps {
		names[i] = s.Name
	}
	return names
}

func TestIntegrationSourceSchemaFKOrder(t *testing.T) {
	uri := startPostgres(t)
	snaps := collectSource(t, uri, "-s", "app")

	names := collectionNames(snaps)
	want := []string{"app.accounts", "app.users", "app.posts"}
	if len(names) != len(want) {
		t.Fatalf("collections %v", names)
	}
	for i, w := range want {
		if names[i] != w {
			t.Fatalf("FK order: got %v want %v (alphabetical would be accounts, posts, users)", names, want)
		}
	}
	if len(snaps[0].Rows) != 2 || len(snaps[1].Rows) != 3 || len(snaps[2].Rows) != 3 {
		t.Fatalf("row counts: accounts=%d users=%d posts=%d",
			len(snaps[0].Rows), len(snaps[1].Rows), len(snaps[2].Rows))
	}
}

func TestIntegrationSourceNativeTypes(t *testing.T) {
	uri := startPostgres(t)
	snaps := collectSource(t, uri, "-s", "app", "app.accounts")
	if len(snaps[0].Rows) == 0 {
		t.Fatal("no rows")
	}
	id := snaps[0].Rows[0]["id"]
	switch id.(type) {
	case int32, int64, int:
		// native integer from pgx, not json float64
	default:
		t.Fatalf("expected native int id, got %T (%v)", id, id)
	}
}

func TestIntegrationSourceDefaultPublicSchema(t *testing.T) {
	uri := startPostgres(t)
	snaps := collectSource(t, uri)

	if len(snaps) != 1 || snaps[0].Name != "public.simple" || len(snaps[0].Rows) != 2 {
		t.Fatalf("got %+v", snaps)
	}
}

func TestIntegrationSourceExplicitTable(t *testing.T) {
	uri := startPostgres(t)
	snaps := collectSource(t, uri, "-s", "app", "app.users")

	if len(snaps) != 1 || snaps[0].Name != "app.users" || len(snaps[0].Rows) != 3 {
		t.Fatalf("got %+v", snaps)
	}
}

func TestIntegrationSourceCustomQuery(t *testing.T) {
	uri := startPostgres(t)
	snaps := collectSource(t, uri, "-s", "app", "app.users", "-q", `SELECT email FROM app.users WHERE email LIKE 'a%'`)

	if len(snaps) != 1 || len(snaps[0].Rows) != 1 {
		t.Fatalf("got %+v", snaps)
	}
	if snaps[0].Rows[0]["email"] != "alice@example.com" {
		t.Fatalf("row %+v", snaps[0].Rows[0])
	}
}

func TestIntegrationSourceSchemaWildcard(t *testing.T) {
	uri := startPostgres(t)
	snaps := collectSource(t, uri, "app.*")

	names := collectionNames(snaps)
	want := []string{"app.accounts", "app.users", "app.posts"}
	for i, w := range want {
		if i >= len(names) || names[i] != w {
			t.Fatalf("wildcard FK order: got %v want %v", names, want)
		}
	}
}

func TestIntegrationSourceComplexTypes(t *testing.T) {
	uri := startPostgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	applyComplexFixtures(t, ctx, uri)

	snaps := collectSource(t, uri, "-s", "complex", "complex.documents")
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
	userTags, ok := user0["tags"].([]any)
	if !ok || len(userTags) != 2 {
		t.Fatalf("user tags %+v", user0["tags"])
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
	uri := startPostgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	applyComplexFixtures(t, ctx, uri)
	execSQL(t, ctx, uri, `
		DROP TABLE IF EXISTS complex.documents_copy;
		CREATE TABLE complex.documents_copy (
			id serial PRIMARY KEY,
			tags text[] NOT NULL,
			payload jsonb NOT NULL
		)`)

	srcSnaps := collectSource(t, uri, "-s", "complex", "complex.documents")
	in := renameCollectionStream(snapshotsToStream(srcSnaps), "complex.documents", "complex.documents_copy")

	sinkOpts, err := pg.ParseSinkOptions(uri, []string{"-s", "complex"})
	if err != nil {
		t.Fatal(err)
	}
	sink := pg.Sink{}
	if err := sink.Sink(ctx, handlersConfig(), in, sinkOpts); err != nil {
		t.Fatal(err)
	}

	mirrorSnaps := collectSource(t, uri, "-s", "complex", "complex.documents_copy")
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

func TestIntegrationSinkTextualJSONCoercion(t *testing.T) {
	uri := startPostgres(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	execSQL(t, ctx, uri, `DROP TABLE IF EXISTS typed_rows`)
	execSQL(t, ctx, uri, `CREATE TABLE typed_rows (
		id INT NOT NULL,
		note TEXT NOT NULL,
		payload JSONB NOT NULL
	)`)

	stream := func(yield func(coll.Collection, error) bool) {
		rows := func(yieldRow func(coll.Row, error) bool) {
			yieldRow(coll.Row{
				"id":      "42",
				"note":    "hello",
				"payload": `{"nested":{"ok":true,"labels":["x","y"]}}`,
			}, nil)
		}
		yield(coll.Collection{Name: "typed_rows", Rows: rows}, nil)
	}

	sinkOpts, err := pg.ParseSinkOptions(uri, nil)
	if err != nil {
		t.Fatal(err)
	}
	sink := pg.Sink{}
	if err := sink.Sink(ctx, handlersConfig(), stream, sinkOpts); err != nil {
		t.Fatal(err)
	}

	snaps := collectSource(t, uri, "typed_rows")
	if len(snaps) != 1 || len(snaps[0].Rows) != 1 {
		t.Fatalf("got %+v", snaps)
	}
	row := snaps[0].Rows[0]
	switch id := row["id"].(type) {
	case int32:
		if id != 42 {
			t.Fatalf("id %v", id)
		}
	case int64:
		if id != 42 {
			t.Fatalf("id %v", id)
		}
	default:
		t.Fatalf("id type %T val %v", row["id"], row["id"])
	}
	payload := row["payload"]
	var nested map[string]any
	switch p := payload.(type) {
	case map[string]any:
		var ok bool
		nested, ok = p["nested"].(map[string]any)
		if !ok {
			t.Fatalf("nested %+v", p["nested"])
		}
	case string:
		var doc map[string]any
		if err := json.Unmarshal([]byte(p), &doc); err != nil {
			t.Fatalf("payload json: %v", err)
		}
		var ok bool
		nested, ok = doc["nested"].(map[string]any)
		if !ok {
			t.Fatalf("nested %+v", doc["nested"])
		}
	default:
		t.Fatalf("payload type %T", payload)
	}
	if nested["ok"] != true {
		t.Fatalf("nested %+v", nested)
	}
}

func TestIntegrationSinkRoundTrip(t *testing.T) {
	uri := startPostgres(t)
	ctx := context.Background()

	srcSnaps := collectSource(t, uri, "-s", "app")
	in := renameSchemaStream(snapshotsToStream(srcSnaps), "app.", "roundtrip.")

	sinkOpts, err := pg.ParseSinkOptions(uri, []string{"--auto-create", "-s", "public"})
	if err != nil {
		t.Fatal(err)
	}
	sink := pg.Sink{}
	if err := sink.Sink(ctx, handlersConfig(), in, sinkOpts); err != nil {
		t.Fatal(err)
	}

	mirrorSnaps := collectSource(t, uri, "-s", "roundtrip")
	if len(mirrorSnaps) != len(srcSnaps) {
		t.Fatalf("mirror len %d src len %d", len(mirrorSnaps), len(srcSnaps))
	}
	for i := range srcSnaps {
		if len(mirrorSnaps[i].Rows) != len(srcSnaps[i].Rows) {
			t.Fatalf("collection %d rows: mirror=%d src=%d", i, len(mirrorSnaps[i].Rows), len(srcSnaps[i].Rows))
		}
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

func renameSchemaStream(in coll.Stream, from, to string) coll.Stream {
	return func(yield func(coll.Collection, error) bool) {
		for c, err := range in {
			if err != nil {
				yield(coll.Collection{}, err)
				return
			}
			c.Name = strings.Replace(c.Name, from, to, 1)
			if !yield(c, nil) {
				return
			}
		}
	}
}
