package xlsx_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ceymard/swl-go/handler/xlsx"
	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/handlers"
	"github.com/ceymard/swl-go/internal/msg"
	"github.com/ceymard/swl-go/internal/pipeline"
	"github.com/ceymard/swl-go/test/swltest"
)

const fixtureXLSX = "../../testdata/xlsx/simple.xlsx"
const fixtureODS = "../../testdata/xlsx/simple.ods"

func TestSourceFixtureXLSXAllSheets(t *testing.T) {
	snaps := collectSource(t, fixtureXLSX, xlsx.SrcOpts{File: fixtureXLSX})
	if len(snaps) != 2 {
		t.Fatalf("collections %d", len(snaps))
	}
	byName := indexSnapshots(snaps)
	if byName["Sheet1"].Rows[0]["name"] != "alice" || byName["Sheet1"].Rows[1]["name"] != "bob" {
		t.Fatalf("Sheet1 rows %+v", byName["Sheet1"].Rows)
	}
	if byName["Sheet2"].Rows[0]["x"] != int64(9) {
		t.Fatalf("Sheet2 %+v", byName["Sheet2"].Rows[0])
	}
}

func TestSourceFixtureXLSXDotColumnInclude(t *testing.T) {
	snaps := collectSource(t, fixtureXLSX, xlsx.SrcOpts{File: fixtureXLSX, Include: true})
	row := snaps[0].Rows[0]
	if row[".secret"] != "hidden" {
		t.Fatalf("expected .secret column, row %+v", row)
	}
	if _, ok := row["_skip"]; ok {
		t.Fatalf("_skip column should be omitted: %+v", row)
	}
}

func TestSourceFixtureXLSXDotColumnExcluded(t *testing.T) {
	snaps := collectSource(t, fixtureXLSX, xlsx.SrcOpts{File: fixtureXLSX})
	row := snaps[0].Rows[0]
	if _, ok := row[".secret"]; ok {
		t.Fatalf(".secret should be excluded without -i: %+v", row)
	}
}

func TestSourceFixtureODS(t *testing.T) {
	if _, err := os.Stat(fixtureODS); err != nil {
		t.Skip("ods fixture missing")
	}
	snaps := collectSource(t, fixtureODS, xlsx.SrcOpts{File: fixtureODS})
	if len(snaps) < 1 || len(snaps[0].Rows) != 2 {
		t.Fatalf("snaps %+v", snaps)
	}
}

func TestSourceXLSXRenameAndSelect(t *testing.T) {
	path := writeTestXLSX(t)
	snaps := collectSource(t, path, xlsx.SrcOpts{
		File: path,
		Sheets: []xlsx.SheetSpec{{
			Name:   "Sheet1",
			Rename: "people",
		}},
	})
	if len(snaps) != 1 || snaps[0].Name != "people" {
		t.Fatalf("snaps %+v", snaps)
	}
}

func TestSourceXLSXSkipUnderscoreSheet(t *testing.T) {
	path := writeTestXLSXWithHiddenSheet(t)
	snaps := collectSource(t, path, xlsx.SrcOpts{File: path})
	for _, s := range snaps {
		if s.Name == "_hidden" {
			t.Fatalf("unexpected _hidden sheet")
		}
	}
}

func TestSourceXLSXTildeNull(t *testing.T) {
	path := writeTestXLSX(t)
	f, err := openExcelizeForTest(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sn := f.GetSheetName(0)
	if err := f.SetCellValue(sn, "B2", "~"); err != nil {
		t.Fatal(err)
	}
	if err := f.Save(); err != nil {
		t.Fatal(err)
	}
	snaps := collectSource(t, path, xlsx.SrcOpts{File: path})
	if snaps[0].Rows[0]["name"] != nil {
		t.Fatalf("expected null name, got %+v", snaps[0].Rows[0]["name"])
	}
}

func TestSourceXLSXSkipsEmptyRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty-row.xlsx")
	f := newExcelizeFile(t)
	sn := f.GetSheetName(0)
	setCell(t, f, sn, "A1", "id")
	setCell(t, f, sn, "A2", 1)
	setCell(t, f, sn, "A4", 2)
	saveExcelize(t, f, path)
	snaps := collectSource(t, path, xlsx.SrcOpts{File: path})
	if len(snaps[0].Rows) != 2 {
		t.Fatalf("rows %d", len(snaps[0].Rows))
	}
}

func TestSourceXLSXFormulaError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "err.xlsx")
	f := newExcelizeFile(t)
	sn := f.GetSheetName(0)
	setCell(t, f, sn, "A1", "id")
	setCell(t, f, sn, "B1", "extra")
	setCell(t, f, sn, "A2", 1)
	setFormula(t, f, sn, "B2", "NA()")
	saveExcelize(t, f, path)
	_, err := runSourceErr(path, xlsx.SrcOpts{File: path})
	if err == nil {
		t.Fatal("expected formula error")
	}
}

func TestSourceXLSXIgnoreErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "err.xlsx")
	f := newExcelizeFile(t)
	sn := f.GetSheetName(0)
	setCell(t, f, sn, "A1", "id")
	setCell(t, f, sn, "B1", "extra")
	setCell(t, f, sn, "A2", 1)
	setFormula(t, f, sn, "B2", "NA()")
	saveExcelize(t, f, path)
	snaps := collectSource(t, path, xlsx.SrcOpts{File: path, IgnoreErrors: true})
	if snaps[0].Rows[0]["id"] != int64(1) {
		t.Fatalf("row %+v", snaps[0].Rows[0])
	}
}

func TestSourceMissingSheet(t *testing.T) {
	path := writeTestXLSX(t)
	_, err := runSourceErr(path, xlsx.SrcOpts{
		File:   path,
		Sheets: []xlsx.SheetSpec{{Name: "Nope"}},
	})
	if err == nil {
		t.Fatal("expected missing sheet error")
	}
}

func TestSourceLegacyXLSRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.xls")
	if err := os.WriteFile(path, []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := runSourceErr(path, xlsx.SrcOpts{File: path})
	if err == nil {
		t.Fatal("expected .xls error")
	}
}

func TestSourceEmptySheetLogs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.xlsx")
	f := newExcelizeFile(t)
	sn := f.GetSheetName(0)
	setCell(t, f, sn, "A1", "id")
	saveExcelize(t, f, path)
	snaps := collectSource(t, path, xlsx.SrcOpts{File: path})
	if len(snaps) != 0 {
		t.Fatalf("expected no collections, got %+v", snaps)
	}
}

func TestSourceMissingFile(t *testing.T) {
	_, err := runSourceErr("/no/such/workbook.xlsx", xlsx.SrcOpts{File: "/no/such/workbook.xlsx"})
	if err == nil {
		t.Fatal("expected open error")
	}
}

func TestPipelineParseXLSXExtension(t *testing.T) {
	p, err := pipeline.Parse([]string{fixtureXLSX}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if p.Stages[0].ID != "xlsx-src" {
		t.Fatalf("stage %s", p.Stages[0].ID)
	}
	opts := p.Stages[0].Options.(xlsx.SrcOpts)
	if opts.File != fixtureXLSX {
		t.Fatalf("file %q", opts.File)
	}
}

func TestPipelineParseXLSBExtension(t *testing.T) {
	p, err := pipeline.Parse([]string{"report.xlsb"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if p.Stages[0].ID != "xlsx-src" {
		t.Fatalf("stage %s", p.Stages[0].ID)
	}
}

func TestPipelineParseODSExtension(t *testing.T) {
	p, err := pipeline.Parse([]string{"report.ods"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if p.Stages[0].ID != "xlsx-src" {
		t.Fatalf("stage %s", p.Stages[0].ID)
	}
}

func TestSourceODSWithLibreOffice(t *testing.T) {
	xlsxPath := writeTestXLSX(t)
	odsPath := convertToODS(t, xlsxPath)
	snaps := collectSource(t, odsPath, xlsx.SrcOpts{File: odsPath})
	if snaps[0].Rows[0]["name"] != "alice" {
		t.Fatalf("row %+v", snaps[0].Rows[0])
	}
}

func collectSource(t *testing.T, path string, opts xlsx.SrcOpts) []swltest.Snapshot {
	t.Helper()
	if opts.File == "" {
		opts.File = path
	}
	stream, err := runSourceStream(path, opts)
	if err != nil {
		t.Fatal(err)
	}
	snaps, err := swltest.CollectStream(t, stream)
	if err != nil {
		t.Fatal(err)
	}
	return snaps
}

func indexSnapshots(snaps []swltest.Snapshot) map[string]swltest.Snapshot {
	m := make(map[string]swltest.Snapshot, len(snaps))
	for _, s := range snaps {
		m[s.Name] = s
	}
	return m
}

func runSourceStream(path string, opts xlsx.SrcOpts) (coll.Stream, error) {
	var src xlsx.Source
	return src.Source(context.Background(), handlers.Config{Messages: msg.New(2)}, opts)
}

func runSourceErr(path string, opts xlsx.SrcOpts) ([]swltest.Snapshot, error) {
	if opts.File == "" {
		opts.File = path
	}
	stream, err := runSourceStream(path, opts)
	if err != nil {
		return nil, err
	}
	var snaps []swltest.Snapshot
	for c, err := range stream {
		if err != nil {
			return nil, err
		}
		snap := swltest.Snapshot{Name: c.Name}
		for row, err := range c.Rows {
			if err != nil {
				return nil, err
			}
			snap.Rows = append(snap.Rows, row)
		}
		snaps = append(snaps, snap)
	}
	return snaps, nil
}

func writeTestXLSX(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sample.xlsx")
	f := newExcelizeFile(t)
	sn := f.GetSheetName(0)
	setCell(t, f, sn, "A1", "id")
	setCell(t, f, sn, "B1", "name")
	setCell(t, f, sn, "A2", 1)
	setCell(t, f, sn, "B2", "alice")
	if _, err := f.NewSheet("Sheet2"); err != nil {
		t.Fatal(err)
	}
	setCell(t, f, "Sheet2", "A1", "x")
	setCell(t, f, "Sheet2", "A2", 9)
	saveExcelize(t, f, path)
	return path
}

func writeTestXLSXWithHiddenSheet(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "hidden.xlsx")
	f := newExcelizeFile(t)
	sn := f.GetSheetName(0)
	setCell(t, f, sn, "A1", "id")
	setCell(t, f, sn, "A2", 1)
	if _, err := f.NewSheet("_hidden"); err != nil {
		t.Fatal(err)
	}
	setCell(t, f, "_hidden", "A1", "secret")
	saveExcelize(t, f, path)
	return path
}

func convertToODS(t *testing.T, xlsxPath string) string {
	t.Helper()
	if _, err := exec.LookPath("soffice"); err != nil {
		t.Skip("soffice not available for ODS conversion")
	}
	dir := t.TempDir()
	cmd := exec.Command("soffice", "--headless", "--convert-to", "ods", "--outdir", dir, xlsxPath)
	if err := cmd.Run(); err != nil {
		t.Skip("libreoffice ods conversion failed:", err)
	}
	base := filepath.Base(xlsxPath)
	out := filepath.Join(dir, base[:len(base)-len(filepath.Ext(base))]+".ods")
	if _, err := os.Stat(out); err != nil {
		t.Skip("ods output missing:", err)
	}
	return out
}
