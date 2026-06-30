package pg_test

import (
	"context"
	"os"
	"path/filepath"
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
