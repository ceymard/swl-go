package xlsx_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ceymard/swl-go/handler/xlsx"
	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/handlers"
	"github.com/ceymard/swl-go/internal/pipeline"
	"github.com/xuri/excelize/v2"
)

func TestParseSinkOptions(t *testing.T) {
	opts, err := xlsx.ParseSinkOptions("out.xlsx", []string{"-u"})
	if err != nil {
		t.Fatal(err)
	}
	o := opts.(xlsx.SinkOpts)
	if o.File != "out.xlsx" || !o.Uncompress {
		t.Fatalf("opts %+v", o)
	}
}

func TestSinkOptParserHelp(t *testing.T) {
	text := xlsx.SinkOptParser().GetHelp("", "swl :: xlsx")
	if text == "" {
		t.Fatal("empty help")
	}
}

func TestPipelineParseXLSXSinkExtension(t *testing.T) {
	p, err := pipeline.Parse([]string{fixtureXLSX, "::", "out.xlsx"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Stages) != 2 || p.Stages[1].ID != "xlsx-sink" {
		t.Fatalf("stages %+v", p.Stages)
	}
	opts := p.Stages[1].Options.(xlsx.SinkOpts)
	if opts.File != "out.xlsx" {
		t.Fatalf("file %q", opts.File)
	}
}

func TestSinkWriteAndReadBack(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.xlsx")

	stream := func(yield func(coll.Collection, error) bool) {
		rows := coll.SliceRows([]coll.Row{
			{"id": int64(1), "name": "alice"},
			{"id": int64(2), "name": "bob"},
		})
		yield(coll.Collection{Name: "people", Rows: rows}, nil)
	}

	sink := xlsx.Sink{}
	opts, _ := xlsx.ParseSinkOptions(out, nil)
	if err := sink.Sink(context.Background(), handlers.Config{}, stream, opts); err != nil {
		t.Fatal(err)
	}

	snaps := collectSource(t, out, xlsx.SrcOpts{File: out})
	if len(snaps) != 1 {
		t.Fatalf("collections %d", len(snaps))
	}
	if snaps[0].Name != "people" || len(snaps[0].Rows) != 2 {
		t.Fatalf("snaps %+v", snaps)
	}
	if snaps[0].Rows[0]["name"] != "alice" {
		t.Fatalf("row %+v", snaps[0].Rows[0])
	}
}

func TestSinkReplacesExistingSheet(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "book.xlsx")

	writeSink(t, out, "people", []coll.Row{{"id": int64(1), "name": "alice"}})
	writeSink(t, out, "people", []coll.Row{{"id": int64(9), "name": "zara"}})

	snaps := collectSource(t, out, xlsx.SrcOpts{File: out})
	if len(snaps[0].Rows) != 1 || snaps[0].Rows[0]["name"] != "zara" {
		t.Fatalf("snaps %+v", snaps)
	}
}

func TestSinkOpensExistingWorkbook(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "book.xlsx")

	f := newExcelizeFile(t)
	setCell(t, f, "Sheet1", "A1", "keep")
	saveExcelize(t, f, out)

	writeSink(t, out, "added", []coll.Row{{"x": int64(1)}})

	wb, err := excelize.OpenFile(out)
	if err != nil {
		t.Fatal(err)
	}
	defer wb.Close()
	if !sheetExistsWorkbook(wb, "Sheet1") || !sheetExistsWorkbook(wb, "added") {
		t.Fatalf("sheets %v", wb.GetSheetList())
	}
}

func TestSinkNormalizesObjectsAndDates(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.xlsx")
	when := time.Date(2024, 3, 15, 10, 30, 0, 0, time.UTC)

	stream := func(yield func(coll.Collection, error) bool) {
		rows := coll.SliceRows([]coll.Row{{
			"when": when,
			"meta": map[string]any{"k": "v"},
		}})
		yield(coll.Collection{Name: "data", Rows: rows}, nil)
	}
	sink := xlsx.Sink{}
	opts, _ := xlsx.ParseSinkOptions(out, nil)
	if err := sink.Sink(context.Background(), handlers.Config{}, stream, opts); err != nil {
		t.Fatal(err)
	}

	f, err := excelize.OpenFile(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	whenCell, _ := f.GetCellValue("data", "B2")
	metaCell, _ := f.GetCellValue("data", "A2")
	if whenCell != when.Format(time.RFC3339) {
		t.Fatalf("when %q", whenCell)
	}
	if metaCell != `{"k":"v"}` {
		t.Fatalf("meta %q", metaCell)
	}
}

func TestSinkRejectsXLSBOutput(t *testing.T) {
	stream := func(yield func(coll.Collection, error) bool) {
		yield(coll.Collection{Name: "x", Rows: coll.SliceRows([]coll.Row{{"a": 1}})}, nil)
	}
	sink := xlsx.Sink{}
	opts, _ := xlsx.ParseSinkOptions("out.xlsb", nil)
	if err := sink.Sink(context.Background(), handlers.Config{}, stream, opts); err == nil {
		t.Fatal("expected error")
	}
}

func TestSinkEmptyCollection(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.xlsx")
	stream := func(yield func(coll.Collection, error) bool) {
		yield(coll.Collection{Name: "empty", Rows: coll.SliceRows(nil)}, nil)
	}
	sink := xlsx.Sink{}
	opts, _ := xlsx.ParseSinkOptions(out, nil)
	if err := sink.Sink(context.Background(), handlers.Config{}, stream, opts); err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenFile(out)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if !sheetExistsWorkbook(f, "empty") {
		t.Fatal("missing empty sheet")
	}
}

func writeSink(t *testing.T, path, sheet string, rows []coll.Row) {
	t.Helper()
	stream := func(yield func(coll.Collection, error) bool) {
		yield(coll.Collection{Name: sheet, Rows: coll.SliceRows(rows)}, nil)
	}
	sink := xlsx.Sink{}
	opts, err := xlsx.ParseSinkOptions(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := sink.Sink(context.Background(), handlers.Config{}, stream, opts); err != nil {
		t.Fatal(err)
	}
}

func sheetExistsWorkbook(f *excelize.File, name string) bool {
	idx, err := f.GetSheetIndex(name)
	return err == nil && idx >= 0
}

func TestSinkRoundTripFromFixture(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "roundtrip.xlsx")

	srcStream, err := runSourceStream(fixtureXLSX, xlsx.SrcOpts{File: fixtureXLSX})
	if err != nil {
		t.Fatal(err)
	}
	sink := xlsx.Sink{}
	opts, _ := xlsx.ParseSinkOptions(out, nil)
	if err := sink.Sink(context.Background(), handlers.Config{}, srcStream, opts); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(out); err != nil {
		t.Fatal(err)
	}
	snaps := collectSource(t, out, xlsx.SrcOpts{File: out})
	if len(snaps) < 2 {
		t.Fatalf("collections %d", len(snaps))
	}
}
