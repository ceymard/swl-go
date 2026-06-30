package mssql_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/ceymard/swl-go/handler/mssql"
	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/handlers"
	"github.com/ceymard/swl-go/internal/msg"
	"github.com/ceymard/swl-go/test/swltest"
	"github.com/testcontainers/testcontainers-go"
	tcmssql "github.com/testcontainers/testcontainers-go/modules/mssql"
	"github.com/testcontainers/testcontainers-go/wait"

	_ "github.com/microsoft/go-mssqldb"
)

var (
	mssqlContainer *tcmssql.MSSQLServerContainer
	mssqlURI       string
)

func TestMain(m *testing.M) {
	code := 1
	defer func() { os.Exit(code) }()
	if os.Getenv("SKIP_TESTCONTAINERS") == "" {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		container, err := tcmssql.Run(ctx,
			"mcr.microsoft.com/mssql/server:2022-CU14-ubuntu-22.04",
			tcmssql.WithAcceptEULA(),
			testcontainers.WithWaitStrategy(
				wait.ForLog("SQL Server is now ready for client connections").WithStartupTimeout(120*time.Second),
			),
		)
		cancel()
		if err == nil {
			mssqlContainer = container
			ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Minute)
			host, _ := container.Host(ctx2)
			port, _ := container.MappedPort(ctx2, "1433/tcp")
			mssqlURI = fmt.Sprintf("server=%s;user id=sa;password=%s;database=master;encrypt=disable",
				host+","+port.Port(), container.Password())
			if err := waitForSQL(ctx2, mssqlURI); err != nil {
				mssqlURI = ""
				mssqlContainer = nil
				_ = testcontainers.TerminateContainer(container)
			}
			cancel2()
		}
	}
	code = m.Run()
	if mssqlContainer != nil {
		_ = testcontainers.TerminateContainer(mssqlContainer)
	}
}

func waitForSQL(ctx context.Context, connStr string) error {
	var last error
	for range 30 {
		db, err := sql.Open("sqlserver", connStr)
		if err != nil {
			last = err
			time.Sleep(time.Second)
			continue
		}
		last = db.PingContext(ctx)
		_ = db.Close()
		if last == nil {
			return nil
		}
		time.Sleep(time.Second)
	}
	return last
}

func handlersConfig() handlers.Config {
	return handlers.Config{Messages: msg.New(0)}
}

func startMSSQL(t *testing.T) string {
	t.Helper()
	if mssqlURI == "" {
		t.Skip("mssql testcontainer unavailable (docker required, or set SKIP_TESTCONTAINERS=1 to skip)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	t.Cleanup(cancel)
	execSQL(t, ctx, mssqlURI, "IF DB_ID(N'swltest') IS NULL CREATE DATABASE swltest")
	dbURI := strings.Replace(mssqlURI, "database=master", "database=swltest", 1)
	applyComplexFixtures(t, ctx, dbURI)
	return dbURI
}

func applyComplexFixtures(t *testing.T, ctx context.Context, connStr string) {
	t.Helper()
	sqlBytes, err := os.ReadFile(complexFixturePath())
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlserver", connStr)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, stmt := range splitSQLBatches(string(sqlBytes)) {
		if stmt == "" {
			continue
		}
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			t.Fatalf("apply complex fixtures: %v\nstmt: %s", err, stmt)
		}
	}
}

func splitSQLBatches(sqlText string) []string {
	parts := strings.Split(sqlText, "\nGO\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 1 {
		return []string{strings.TrimSpace(sqlText)}
	}
	return out
}

func complexFixturePath() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", "mssql", "complex_types.sql")
}

func execSQL(t *testing.T, ctx context.Context, connStr, stmt string) {
	t.Helper()
	db, err := sql.Open("sqlserver", connStr)
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
	opts, err := mssql.ParseSrcOptions(uri, tail)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := mssql.Source{}.Source(context.Background(), handlersConfig(), opts)
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
	uri := startMSSQL(t)
	snaps := collectSource(t, uri, "dbo.documents")
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
	uri := startMSSQL(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	execSQL(t, ctx, uri, "IF OBJECT_ID(N'dbo.documents_copy', N'U') IS NOT NULL DROP TABLE dbo.documents_copy")
	execSQL(t, ctx, uri, `CREATE TABLE dbo.documents_copy (
		id INT IDENTITY(1,1) PRIMARY KEY,
		tags NVARCHAR(MAX) NOT NULL,
		payload NVARCHAR(MAX) NOT NULL
	)`)

	srcSnaps := collectSource(t, uri, "dbo.documents")
	in := renameCollectionStream(snapshotsToStream(srcSnaps), "dbo.documents", "dbo.documents_copy")

	sinkOpts, err := mssql.ParseSinkOptions(uri, nil)
	if err != nil {
		t.Fatal(err)
	}
	sink := mssql.Sink{}
	if err := sink.Sink(ctx, handlersConfig(), in, sinkOpts); err != nil {
		t.Fatal(err)
	}

	mirrorSnaps := collectSource(t, uri, "dbo.documents_copy")
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
	uri := startMSSQL(t)
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
		yield(coll.Collection{Name: "dbo.created_docs", Rows: rows}, nil)
	}

	sinkOpts, err := mssql.ParseSinkOptions(uri, []string{"--auto-create"})
	if err != nil {
		t.Fatal(err)
	}
	sink := mssql.Sink{}
	if err := sink.Sink(ctx, handlersConfig(), stream, sinkOpts); err != nil {
		t.Fatal(err)
	}

	snaps := collectSource(t, uri, "dbo.created_docs")
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
