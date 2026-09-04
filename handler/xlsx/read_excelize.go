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

// readSheet streams the sheet row by row instead of materializing it into a
// [][]string table upfront (excelize.GetRows) — a sheet with no tabular
// header row (or one excluded by spec) is abandoned after its first row,
// without decoding the rest of its XML.
func (b *excelizeBook) readSheet(spec SheetSpec) ([]coll.Row, *coll.ColumnSet, error) {
	if _, err := b.file.GetSheetIndex(spec.Name); err != nil {
		return nil, nil, errs.New(`no such sheet "` + spec.Name + `"`)
	}

	rows, err := b.file.Rows(spec.Name)
	if err != nil {
		return nil, nil, errs.Wrap(err, "read sheet", "sheet", spec.Name)
	}
	defer rows.Close()

	if !rows.Next() {
		return nil, nil, nil // empty sheet
	}
	headerRow, err := b.rowValues(rows, spec.Name, 1)
	if err != nil {
		return nil, nil, err
	}
	headers := readHeaders(headerRow, spec.Include)
	if len(headers) == 0 {
		return nil, nil, nil // not tabular — bail without reading further rows
	}
	indexes := headerIndexes(headerRow, headers, spec.Include)
	cs := coll.NewColumnSet()
	for _, h := range headers {
		cs.Index(h)
	}

	var out []coll.Row
	rowNum := 1
	for rows.Next() {
		rowNum++
		values, err := b.rowValues(rows, spec.Name, rowNum)
		if err != nil {
			return nil, nil, err
		}
		row, found, err := buildRow(values, indexes, headers, spec, rowNum, columnName)
		if err != nil {
			return nil, nil, err
		}
		if found {
			out = append(out, row)
		}
	}
	if err := rows.Error(); err != nil {
		return nil, nil, errs.Wrap(err, "read sheet", "sheet", spec.Name)
	}
	return out, cs, nil
}

// rowValues returns rowNum's raw cell values off the streaming iterator,
// resolving any formula cell that has no cached value — the same
// GetCellFormula/CalcCellValue fallback GetRows used to apply over the
// whole sheet upfront, now done per row as rows are actually consumed.
func (b *excelizeBook) rowValues(rows *excelize.Rows, sheet string, rowNum int) ([]string, error) {
	values, err := rows.Columns(excelize.Options{RawCellValue: true})
	if err != nil {
		return nil, errs.Wrap(err, "read sheet", "sheet", sheet)
	}
	for c, v := range values {
		if v != "" {
			continue
		}
		coord, err := excelize.CoordinatesToCellName(c+1, rowNum)
		if err != nil {
			continue
		}
		formula, _ := b.file.GetCellFormula(sheet, coord)
		if formula == "" {
			continue
		}
		calc, err := b.file.CalcCellValue(sheet, coord)
		if err != nil {
			values[c] = "#ERROR"
			continue
		}
		if calc != "" {
			values[c] = calc
		}
	}
	return values, nil
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
