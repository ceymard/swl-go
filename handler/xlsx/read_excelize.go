package xlsx

import (
	"strconv"
	"strings"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/errs"
	"github.com/xuri/excelize/v2"
)

type excelizeBook struct {
	file *excelize.File
}

func openExcelize(path string) (sheetReader, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, errs.Wrap(err, "open spreadsheet", "path", path)
	}
	return &excelizeBook{file: f}, nil
}

func (b *excelizeBook) Close() error {
	if b.file == nil {
		return nil
	}
	return b.file.Close()
}

func (b *excelizeBook) sheetSpecs(opts SrcOpts) ([]SheetSpec, error) {
	return resolveSheetSpecs(b.file.GetSheetList(), opts), nil
}

func (b *excelizeBook) readSheet(spec SheetSpec) ([]coll.Row, *coll.ColumnSet, error) {
	if _, err := b.file.GetSheetIndex(spec.Name); err != nil {
		return nil, nil, errs.New(`no such sheet "` + spec.Name + `"`)
	}

	table, err := b.sheetTable(spec.Name)
	if err != nil {
		return nil, nil, err
	}
	return rowsFromTable(table, spec, columnName)
}

func (b *excelizeBook) sheetTable(sheet string) ([][]string, error) {
	rows, err := b.file.GetRows(sheet, excelize.Options{RawCellValue: true})
	if err != nil {
		return nil, errs.Wrap(err, "read sheet", "sheet", sheet)
	}
	if len(rows) == 0 {
		return nil, nil
	}

	out := make([][]string, len(rows))
	for r, row := range rows {
		out[r] = append([]string(nil), row...)
		for c := range row {
			coord, err := excelize.CoordinatesToCellName(c+1, r+1)
			if err != nil {
				continue
			}
			// Use cached values from the file when present (swl2/xlsx behavior).
			// Only recalculate formula cells that have no cached value.
			if out[r][c] != "" {
				continue
			}
			formula, _ := b.file.GetCellFormula(sheet, coord)
			if formula == "" {
				continue
			}
			calc, err := b.file.CalcCellValue(sheet, coord)
			if err != nil {
				out[r][c] = "#ERROR"
				continue
			}
			if calc != "" {
				out[r][c] = calc
			}
		}
	}
	return out, nil
}

func parseInt(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, strconv.ErrSyntax
	}
	return strconv.ParseInt(s, 10, 64)
}

func parseFloat(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, strconv.ErrSyntax
	}
	return strconv.ParseFloat(s, 64)
}
