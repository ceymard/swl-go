package csv

import (
	"compress/gzip"
	"context"
	"encoding/csv"
	"io"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/errs"
	"github.com/ceymard/swl-go/internal/handlers"
)

// Source reads one or more CSV files into collections.
type Source struct{}

func (Source) Source(ctx context.Context, cfg handlers.Config, raw any) (coll.Stream, error) {
	opts := raw.(SrcOpts)
	if opts.Encoding != "" && opts.Encoding != "utf-8" {
		return nil, errs.New("csv source: only utf-8 encoding is supported")
	}

	files := append([]string(nil), opts.Files...)
	sort.Strings(files)

	type group struct {
		name  string
		files []string
	}
	byName := map[string][]string{}
	var order []string
	for _, f := range files {
		name := collectionName(f, opts.Collection)
		if _, ok := byName[name]; !ok {
			order = append(order, name)
		}
		byName[name] = append(byName[name], f)
	}

	var groups []group
	for _, name := range order {
		groups = append(groups, group{name: name, files: byName[name]})
	}

	return func(yield func(coll.Collection, error) bool) {
		for _, g := range groups {
			cs := coll.NewColumnSet()
			c := coll.Collection{
				Name:    g.name,
				Columns: cs,
				Rows:    readFiles(ctx, opts, cs, g.files),
			}
			if !yield(c, nil) {
				return
			}
		}
	}, nil
}

func readFiles(ctx context.Context, opts SrcOpts, cs *coll.ColumnSet, files []string) coll.RowBatches {
	return func(yield func([]coll.Row, error) bool) {
		batch := make([]coll.Row, 0, coll.DefaultBatchSize)
		stopped := false
		flush := func() bool {
			if stopped || len(batch) == 0 {
				return !stopped
			}
			ok := yield(batch, nil)
			batch = make([]coll.Row, 0, coll.DefaultBatchSize)
			if !ok {
				stopped = true
			}
			return ok
		}
		appendRow := func(row coll.Row) bool {
			if stopped {
				return false
			}
			batch = append(batch, row)
			if len(batch) == coll.DefaultBatchSize {
				return flush()
			}
			return true
		}
		for _, path := range files {
			if stopped {
				return
			}
			if err := ctx.Err(); err != nil {
				yield(nil, err)
				return
			}
			if err := readFile(ctx, opts, cs, path, appendRow); err != nil {
				yield(nil, err)
				return
			}
		}
		if !stopped {
			flush()
		}
	}
}

func readFile(ctx context.Context, opts SrcOpts, cs *coll.ColumnSet, path string, appendRow func(coll.Row) bool) error {
	f, err := os.Open(path)
	if err != nil {
		return errs.Wrap(err, "open csv file", "path", path)
	}
	defer f.Close()

	var r io.Reader = f
	if opts.Gunzip || strings.HasSuffix(strings.ToLower(path), ".gz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return errs.Wrap(err, "gunzip csv", "path", path)
		}
		defer gz.Close()
		r = gz
	}

	reader := csv.NewReader(r)
	if len(opts.Delimiter) > 0 {
		reader.Comma = rune(opts.Delimiter[0])
	}
	if opts.Quote != 0 {
		reader.LazyQuotes = true
	}

	var headers []string
	if len(opts.Headers) > 0 {
		headers = opts.Headers
	} else {
		record, err := reader.Read()
		if err != nil {
			return errs.Wrap(err, "read csv headers", "path", path)
		}
		headers = record
		if opts.SimplifyHeaders {
			for i, h := range headers {
				headers[i] = simplifyHeader(h)
			}
		}
	}

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		record, err := reader.Read()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return errs.Wrap(err, "read csv row", "path", path)
		}
		row := recordToRow(cs, headers, record)
		row = applyMerge(cs, row, opts.Merge)
		applyNumbers(row, opts.Numbers)
		applyEmpty(row, opts.NoEmpty, opts.EmptyIsNull)
		if !appendRow(row) {
			return nil
		}
	}
}

// recordToRow builds a row positionally against cs, registering each
// header (in file order) the first time it's seen. Headers repeated across
// files feeding the same collection resolve to their already-assigned
// index, keeping every row in this collection aligned.
func recordToRow(cs *coll.ColumnSet, headers, record []string) coll.Row {
	row := make(coll.Row, cs.Len())
	for i, val := range record {
		key := ""
		if i < len(headers) {
			key = headers[i]
		}
		if key == "" {
			key = strconv.Itoa(i)
		}
		idx := cs.Index(key)
		if idx >= len(row) {
			grown := make(coll.Row, idx+1)
			copy(grown, row)
			row = grown
		}
		row[idx] = val
	}
	return row
}

// applyMerge fills in default values for keys absent from row. merge is a
// plain Go map (unordered) — since these are constant/default values with
// no natural source order, any relative index assigned among merge-only
// keys is incidental, not a documented ordering guarantee.
func applyMerge(cs *coll.ColumnSet, row coll.Row, merge map[string]any) coll.Row {
	for k, v := range merge {
		idx := cs.Index(k)
		if idx >= len(row) {
			grown := make(coll.Row, idx+1)
			copy(grown, row)
			row = grown
		}
		if row[idx] == nil {
			row[idx] = v
		}
	}
	return row
}

func applyNumbers(row coll.Row, on bool) {
	if !on {
		return
	}
	for i, v := range row {
		s, ok := v.(string)
		if !ok || s == "" {
			continue
		}
		if n, err := strconv.ParseInt(s, 10, 64); err == nil {
			row[i] = n
			continue
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			row[i] = f
		}
	}
}

// applyEmpty blanks empty-string cells to nil under either flag. A
// positional row can't represent "field truly absent" versus "field
// present but null" for a non-trailing column (removing a middle element
// would shift every later column's meaning) — both NoEmpty and EmptyIsNull
// collapse to the same nil-out here, matching how Row.Cell already treats
// "nil" and "not grown that far" identically downstream.
func applyEmpty(row coll.Row, noEmpty, emptyNull bool) {
	if !noEmpty && !emptyNull {
		return
	}
	for i, v := range row {
		if s, ok := v.(string); ok && s == "" {
			row[i] = nil
		}
	}
}
