package xlsx

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/errs"
	"github.com/ceymard/swl-go/internal/handlers"
)

// Source reads Excel (.xlsx/.xlsb/.xlsm) and ODS spreadsheets.
type Source struct{}

func (Source) Source(ctx context.Context, cfg handlers.Config, raw any) (coll.Stream, error) {
	opts := raw.(SrcOpts)
	if opts.File == "" {
		return nil, errs.New("xlsx source requires a file path")
	}

	wb, err := openWorkbook(opts.File)
	if err != nil {
		return nil, err
	}

	specs, err := wb.sheetSpecs(opts)
	if err != nil {
		wb.Close()
		return nil, err
	}

	return func(yield func(coll.Collection, error) bool) {
		defer wb.Close()
		for _, spec := range specs {
			if skipSheet(spec) {
				continue
			}
			if err := ctx.Err(); err != nil {
				yield(coll.Collection{}, err)
				return
			}

			rows, err := wb.readSheet(spec)
			if err != nil {
				yield(coll.Collection{}, err)
				return
			}
			emitName := collectionName(spec)
			if len(rows) == 0 {
				if cfg.Messages != nil {
					cfg.Messages.Log(2, fmt.Sprintf("%s was empty, nothing emitted", emitName))
				}
				continue
			}
			c := coll.Collection{
				Name: emitName,
				Rows: coll.SliceRows(rows),
			}
			if !yield(c, nil) {
				return
			}
		}
	}, nil
}

func resolveSheetSpecs(names []string, opts SrcOpts) []SheetSpec {
	if len(opts.Sheets) > 0 {
		out := make([]SheetSpec, len(opts.Sheets))
		copy(out, opts.Sheets)
		return out
	}
	out := make([]SheetSpec, 0, len(names))
	for _, name := range names {
		out = append(out, SheetSpec{
			Name:         name,
			Rename:       name,
			IgnoreErrors: opts.IgnoreErrors,
			Include:      opts.Include,
		})
	}
	return out
}

func collectionName(spec SheetSpec) string {
	if spec.Rename != "" {
		return spec.Rename
	}
	return spec.Name
}

func skipSheet(spec SheetSpec) bool {
	return strings.HasPrefix(collectionName(spec), "_")
}

func readHeaders(row []string, include bool) []string {
	var headers []string
	for _, h := range row {
		if h == "" {
			break
		}
		if strings.HasPrefix(h, "_") {
			continue
		}
		if !include && strings.HasPrefix(h, ".") {
			continue
		}
		headers = append(headers, h)
	}
	return headers
}

func headerIndexes(headerRow []string, headers []string, include bool) []int {
	indexes := make([]int, 0, len(headers))
	for i, h := range headerRow {
		if h == "" {
			break
		}
		if strings.HasPrefix(h, "_") {
			continue
		}
		if !include && strings.HasPrefix(h, ".") {
			continue
		}
		for _, want := range headers {
			if want == h {
				indexes = append(indexes, i)
				break
			}
		}
	}
	return indexes
}

func rowsFromTable(table [][]string, spec SheetSpec, colName func(int) string) ([]coll.Row, error) {
	if len(table) == 0 {
		return nil, nil
	}
	headers := readHeaders(table[0], spec.Include)
	if len(headers) == 0 {
		return nil, nil
	}
	indexes := headerIndexes(table[0], headers, spec.Include)

	var out []coll.Row
	for line := 1; line < len(table); line++ {
		rowNum := line + 1
		values := table[line]
		row, found, err := buildRow(values, indexes, headers, spec, rowNum, colName)
		if err != nil {
			return nil, err
		}
		if found {
			out = append(out, row)
		}
	}
	return out, nil
}

func buildRow(values []string, indexes []int, headers []string, spec SheetSpec, rowNum int, colName func(int) string) (coll.Row, bool, error) {
	row := make(coll.Row, len(headers))
	found := false
	for j, colIdx := range indexes {
		head := headers[j]
		val := ""
		if colIdx < len(values) {
			val = values[colIdx]
		}
		cell, isErr, errText := classifyCell(val)
		if isErr {
			if spec.IgnoreErrors {
				cell = nil
			} else {
				ref := colName(colIdx) + itoa(rowNum)
				return nil, false, errs.New("the cell " + ref + " (" + head + ") contained an error: " + errText)
			}
		}
		row[head] = cell
		if !isEmpty(cell) {
			found = true
		}
	}
	return row, found, nil
}

func classifyCell(raw string) (any, bool, string) {
	if raw == "~" {
		return nil, false, ""
	}
	if strings.HasPrefix(raw, "#") {
		return raw, true, raw
	}
	return parseScalar(raw), false, ""
}

func parseScalar(s string) any {
	if s == "" {
		return nil
	}
	switch strings.ToUpper(s) {
	case "TRUE":
		return true
	case "FALSE":
		return false
	}
	if i, err := parseInt(s); err == nil {
		return i
	}
	if f, err := parseFloat(s); err == nil {
		return f
	}
	return s
}

func isEmpty(v any) bool {
	if v == nil {
		return true
	}
	if s, ok := v.(string); ok && s == "" {
		return true
	}
	return false
}

func openWorkbook(path string) (sheetReader, error) {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".ods":
		return openODS(path)
	case ".xlsb":
		return openXLSB(path)
	case ".xls":
		return nil, errs.New("legacy .xls (BIFF) is not supported; use .xlsx or .xlsb")
	default:
		return openExcelize(path)
	}
}

type sheetReader interface {
	Close() error
	sheetSpecs(opts SrcOpts) ([]SheetSpec, error)
	readSheet(spec SheetSpec) ([]coll.Row, error)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func columnName(col int) string {
	const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
	name := ""
	for col >= 0 {
		name = string(letters[col%26]) + name
		col = col/26 - 1
	}
	return name
}
