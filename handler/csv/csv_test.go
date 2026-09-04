package csv_test

import (
	"context"
	stdcsv "encoding/csv"
	"os"
	"path/filepath"
	"testing"

	csvhandler "github.com/ceymard/swl-go/handler/csv"
	"github.com/ceymard/swl-go/internal/handlers"
	"github.com/ceymard/swl-go/internal/msg"
	"github.com/ceymard/swl-go/test/swltest"
)

func TestSourceSimple(t *testing.T) {
	path := filepath.Join("..", "..", "testdata", "csv", "simple.csv")
	opts, err := csvhandler.ParseSrcOptions(path, []string{"-u"})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := csvhandler.Source{}.Source(context.Background(), handlers.Config{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	snaps, err := swltest.CollectStream(t, stream)
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 1 || snaps[0].Name != "simple" {
		t.Fatalf("collection %+v", snaps)
	}
	if len(snaps[0].Rows) != 2 {
		t.Fatalf("rows %d", len(snaps[0].Rows))
	}
	if _, ok := snaps[0].Cell(0, "id").(int64); !ok {
		t.Fatalf("expected numeric id, got %T", snaps[0].Cell(0, "id"))
	}
}

func TestSinkRoundTrip(t *testing.T) {
	inPath := filepath.Join("..", "..", "testdata", "csv", "simple.csv")
	outPath := filepath.Join(t.TempDir(), "out.csv")

	srcOpts, _ := csvhandler.ParseSrcOptions(inPath, nil)
	in, err := csvhandler.Source{}.Source(context.Background(), handlers.Config{}, srcOpts)
	if err != nil {
		t.Fatal(err)
	}

	sinkOpts, _ := csvhandler.ParseSinkOptions(outPath, []string{"-d=,"})
	sink := csvhandler.Sink{}
	if err := sink.Sink(context.Background(), handlers.Config{Messages: msg.New(0)}, in, sinkOpts); err != nil {
		t.Fatal(err)
	}

	f, err := os.Open(outPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := stdcsv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 { // header + 2 data
		t.Fatalf("rows %d", len(rows))
	}
}

func TestMultiFileSource(t *testing.T) {
	dir := t.TempDir()
	writeCSV(t, filepath.Join(dir, "a.csv"), "id\n1\n")
	writeCSV(t, filepath.Join(dir, "b.csv"), "id\n2\n")

	opts, err := csvhandler.ParseSrcOptions(filepath.Join(dir, "a.csv"), []string{"-c", "data", filepath.Join(dir, "b.csv")})
	if err != nil {
		t.Fatal(err)
	}
	stream, err := csvhandler.Source{}.Source(context.Background(), handlers.Config{}, opts)
	if err != nil {
		t.Fatal(err)
	}
	snaps, err := swltest.CollectStream(t, stream)
	if err != nil {
		t.Fatal(err)
	}
	if len(snaps) != 1 || len(snaps[0].Rows) != 2 {
		t.Fatalf("got %+v", snaps)
	}
}

func writeCSV(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
