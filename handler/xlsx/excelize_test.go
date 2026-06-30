package xlsx_test

import (
	"testing"

	"github.com/xuri/excelize/v2"
)

func newExcelizeFile(t *testing.T) *excelize.File {
	t.Helper()
	f := excelize.NewFile()
	t.Cleanup(func() { _ = f.Close() })
	return f
}

func openExcelizeForTest(path string) (*excelize.File, error) {
	return excelize.OpenFile(path)
}

func setCell(t *testing.T, f *excelize.File, sheet, coord string, v any) {
	t.Helper()
	if err := f.SetCellValue(sheet, coord, v); err != nil {
		t.Fatal(err)
	}
}

func setFormula(t *testing.T, f *excelize.File, sheet, coord, formula string) {
	t.Helper()
	if err := f.SetCellFormula(sheet, coord, formula); err != nil {
		t.Fatal(err)
	}
}

func saveExcelize(t *testing.T, f *excelize.File, path string) {
	t.Helper()
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}
}
