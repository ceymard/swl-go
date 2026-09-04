package xlsx

import (
	"math"
	"strconv"

	xlsb "github.com/TsubasaBE/go-xlsb"
	"github.com/TsubasaBE/go-xlsb/workbook"
	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/errs"
)

type xlsbBook struct {
	wb *workbook.Workbook
}

func openXLSB(path string) (sheetReader, error) {
	wb, err := xlsb.Open(path)
	if err != nil {
		return nil, errs.Wrap(err, "open spreadsheet", "path", path)
	}
	return &xlsbBook{wb: wb}, nil
}

func (b *xlsbBook) Close() error {
	if b.wb == nil {
		return nil
	}
	return b.wb.Close()
}

func (b *xlsbBook) sheetSpecs(opts SrcOpts) ([]SheetSpec, error) {
	return resolveSheetSpecs(b.wb.Sheets(), opts), nil
}

func (b *xlsbBook) readSheet(spec SheetSpec) ([]coll.Row, *coll.ColumnSet, error) {
	if _, err := b.wb.SheetByName(spec.Name); err != nil {
		return nil, nil, errs.New(`no such sheet "` + spec.Name + `"`)
	}
	table, err := b.sheetTable(spec.Name)
	if err != nil {
		return nil, nil, err
	}
	return rowsFromTable(table, spec, columnName)
}

func (b *xlsbBook) sheetTable(sheetName string) ([][]string, error) {
	ws, err := b.wb.SheetByName(sheetName)
	if err != nil {
		return nil, errs.Wrap(err, "read sheet", "sheet", sheetName)
	}

	var table [][]string
	for rowCells := range ws.Rows(false) {
		maxCol := 0
		for _, c := range rowCells {
			if c.C+1 > maxCol {
				maxCol = c.C + 1
			}
		}
		line := make([]string, maxCol)
		for _, c := range rowCells {
			line[c.C] = b.cellString(c.V, c.Style)
		}
		table = append(table, line)
	}
	if ws.Err != nil {
		return nil, errs.Wrap(ws.Err, "read sheet", "sheet", sheetName)
	}
	return table, nil
}

func (b *xlsbBook) cellString(v any, style int) string {
	if v == nil {
		return ""
	}
	if f, ok := v.(float64); ok && b.wb.Styles.IsDate(style) {
		return b.wb.FormatCell(f, style)
	}
	switch x := v.(type) {
	case string:
		return x
	case bool:
		if x {
			return "TRUE"
		}
		return "FALSE"
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return ""
		}
		if x == math.Trunc(x) && x >= math.MinInt64 && x <= math.MaxInt64 {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return ""
	}
}
