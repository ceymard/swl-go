package xlsx

import (
	"iter"
	"math"
	"strconv"

	xlsb "github.com/TsubasaBE/go-xlsb"
	"github.com/TsubasaBE/go-xlsb/workbook"
	"github.com/TsubasaBE/go-xlsb/worksheet"
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

// readSheet streams ws.Rows(false) row by row instead of first materializing
// the whole sheet into a [][]string table — a sheet with no tabular header
// row (or one excluded by spec) is abandoned after its first row, without
// draining the rest of the channel (draining continues under the hood via
// the deferred cancel below only if the underlying reader supports it; here
// we simply stop reading, which is safe since ws.Rows is per-call).
func (b *xlsbBook) readSheet(spec SheetSpec) ([]coll.Row, *coll.ColumnSet, error) {
	ws, err := b.wb.SheetByName(spec.Name)
	if err != nil {
		return nil, nil, errs.New(`no such sheet "` + spec.Name + `"`)
	}

	next, stop := iter.Pull(ws.Rows(false))
	defer stop()

	firstRow, ok := next()
	if !ok {
		if ws.Err != nil {
			return nil, nil, errs.Wrap(ws.Err, "read sheet", "sheet", spec.Name)
		}
		return nil, nil, nil // empty sheet
	}
	headerRow := b.rowLine(firstRow)
	headers := readHeaders(headerRow, spec.Include)
	if len(headers) == 0 {
		return nil, nil, nil // not tabular — bail without draining the rest
	}
	indexes := headerIndexes(headerRow, headers, spec.Include)
	cs := coll.NewColumnSet()
	for _, h := range headers {
		cs.Index(h)
	}

	var out []coll.Row
	rowNum := 1
	for {
		rowCells, ok := next()
		if !ok {
			break
		}
		rowNum++
		values := b.rowLine(rowCells)
		row, found, err := buildRow(values, indexes, headers, spec, rowNum, columnName)
		if err != nil {
			return nil, nil, err
		}
		if found {
			out = append(out, row)
		}
	}
	if ws.Err != nil {
		return nil, nil, errs.Wrap(ws.Err, "read sheet", "sheet", spec.Name)
	}
	return out, cs, nil
}

// rowLine converts one xlsb row's sparse cells into a dense []string,
// column-indexed exactly like the old sheetTable's per-row build.
func (b *xlsbBook) rowLine(rowCells []worksheet.Cell) []string {
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
	return line
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
