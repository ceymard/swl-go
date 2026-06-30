package xlsx

import (
	"context"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/errs"
	"github.com/ceymard/swl-go/internal/handlers"
	"github.com/ceymard/swl-go/internal/jsonx"
	"github.com/xuri/excelize/v2"
)

// Sink writes collections to an Excel workbook (swl2 swl-xlsx-sink.ts).
type Sink struct{}

func (Sink) Sink(ctx context.Context, cfg handlers.Config, in coll.Stream, raw any) error {
	opts := raw.(SinkOpts)
	if opts.File == "" {
		return errs.New("xlsx sink requires a file path")
	}
	if err := validateSinkExt(opts.File); err != nil {
		return err
	}

	f, existed, err := openSinkWorkbook(opts.File)
	if err != nil {
		return err
	}
	defer f.Close()
	if existed && cfg.Messages != nil {
		cfg.Log(2, "opened existing file")
	}

	for c, err := range in {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rows, err := collectSinkRows(c.Rows)
		if err != nil {
			return err
		}
		if err := replaceSheet(f, c.Name, rows); err != nil {
			return errs.Wrap(err, "write sheet", "sheet", c.Name)
		}
	}

	if err := f.SaveAs(opts.File); err != nil {
		return errs.Wrap(err, "save spreadsheet", "path", opts.File)
	}
	return nil
}

func collectSinkRows(rows iter.Seq2[coll.Row, error]) ([]coll.Row, error) {
	var out []coll.Row
	for row, err := range rows {
		if err != nil {
			return nil, err
		}
		out = append(out, normalizeSinkRow(row))
	}
	return out, nil
}

func validateSinkExt(path string) error {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".xlsx", ".xlsm", ".xlam", ".xltx", ".xltm":
		return nil
	case ".xlsb":
		return errs.New("xlsx sink cannot write .xlsb files; use .xlsx or .xlsm")
	case ".ods":
		return errs.New("xlsx sink cannot write .ods files; use .xlsx or .xlsm")
	case ".xls":
		return errs.New("xlsx sink cannot write legacy .xls files; use .xlsx or .xlsm")
	default:
		return errs.New("xlsx sink requires a .xlsx or .xlsm output path")
	}
}

func openSinkWorkbook(path string) (*excelize.File, bool, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			f := excelize.NewFile()
			return f, false, nil
		}
		return nil, false, errs.Wrap(err, "stat spreadsheet", "path", path)
	}
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, false, errs.Wrap(err, "open spreadsheet", "path", path)
	}
	return f, true, nil
}

func replaceSheet(f *excelize.File, name string, rows []coll.Row) error {
	if err := deleteSheetIfPresent(f, name); err != nil {
		return err
	}
	if _, err := f.NewSheet(name); err != nil {
		return err
	}
	return writeSheetRows(f, name, rows)
}

func deleteSheetIfPresent(f *excelize.File, name string) error {
	idx, err := f.GetSheetIndex(name)
	if err != nil || idx < 0 {
		return nil
	}
	sheets := f.GetSheetList()
	if len(sheets) == 1 {
		tmp := "_swl_tmp"
		for i := 0; i < 100 && sheetExists(f, tmp); i++ {
			tmp = fmt.Sprintf("_swl_tmp_%d", i)
		}
		if _, err := f.NewSheet(tmp); err != nil {
			return err
		}
	}
	return f.DeleteSheet(name)
}

func sheetExists(f *excelize.File, name string) bool {
	idx, err := f.GetSheetIndex(name)
	return err == nil && idx >= 0
}

func writeSheetRows(f *excelize.File, name string, rows []coll.Row) error {
	cols := sinkColumnNames(rows)
	for j, col := range cols {
		coord, err := excelize.CoordinatesToCellName(j+1, 1)
		if err != nil {
			return err
		}
		if err := f.SetCellValue(name, coord, col); err != nil {
			return err
		}
	}
	for i, row := range rows {
		for j, col := range cols {
			coord, err := excelize.CoordinatesToCellName(j+1, i+2)
			if err != nil {
				return err
			}
			if err := f.SetCellValue(name, coord, row[col]); err != nil {
				return err
			}
		}
	}
	return nil
}

func sinkColumnNames(rows []coll.Row) []string {
	seen := make(map[string]struct{}, 16)
	cols := make([]string, 0, 16)
	for _, row := range rows {
		for k := range row {
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			cols = append(cols, k)
		}
	}
	sort.Strings(cols)
	return cols
}

func normalizeSinkRow(row coll.Row) coll.Row {
	out := make(coll.Row, len(row))
	for k, v := range row {
		out[k] = sinkCellValue(v)
	}
	return out
}

func sinkCellValue(v any) any {
	if v == nil {
		return nil
	}
	switch x := v.(type) {
	case string, bool,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64:
		return v
	case time.Time:
		return x.Format(time.RFC3339)
	default:
		b, err := jsonx.Marshal(v)
		if err != nil {
			return fmt.Sprint(v)
		}
		return string(b)
	}
}
