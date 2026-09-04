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
			c := coll.Collection{
				Name: g.name,
				Rows: readFiles(ctx, opts, g.files),
			}
			if !yield(c, nil) {
				return
			}
		}
	}, nil
}

func readFiles(ctx context.Context, opts SrcOpts, files []string) coll.RowBatches {
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
			if err := readFile(ctx, opts, path, appendRow); err != nil {
				yield(nil, err)
				return
			}
		}
		if !stopped {
			flush()
		}
	}
}

func readFile(ctx context.Context, opts SrcOpts, path string, appendRow func(coll.Row) bool) error {
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
		row := recordToRow(headers, record)
		applyMerge(row, opts.Merge)
		applyNumbers(row, opts.Numbers)
		applyEmpty(row, opts.NoEmpty, opts.EmptyIsNull)
		if !appendRow(row) {
			return nil
		}
	}
}

func recordToRow(headers, record []string) coll.Row {
	row := make(coll.Row, len(record))
	for i, val := range record {
		key := ""
		if i < len(headers) {
			key = headers[i]
		}
		if key == "" {
			key = strconv.Itoa(i)
		}
		row[key] = val
	}
	return row
}

func applyMerge(row coll.Row, merge map[string]any) {
	for k, v := range merge {
		if _, ok := row[k]; !ok {
			row[k] = v
		}
	}
}

func applyNumbers(row coll.Row, on bool) {
	if !on {
		return
	}
	for k, v := range row {
		s, ok := v.(string)
		if !ok || s == "" {
			continue
		}
		if i, err := strconv.ParseInt(s, 10, 64); err == nil {
			row[k] = i
			continue
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			row[k] = f
		}
	}
}

func applyEmpty(row coll.Row, noEmpty, emptyNull bool) {
	if noEmpty {
		for k, v := range row {
			if s, ok := v.(string); ok && s == "" {
				delete(row, k)
			}
		}
		return
	}
	if emptyNull {
		for k, v := range row {
			if s, ok := v.(string); ok && s == "" {
				row[k] = nil
			}
		}
	}
}
