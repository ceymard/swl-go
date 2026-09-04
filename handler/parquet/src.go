package parquet

import (
	"context"
	"io"
	"os"
	"sort"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/errs"
	"github.com/ceymard/swl-go/internal/handlers"
	"github.com/parquet-go/parquet-go"
)

const readBatchSize = 2048

// Source reads Parquet files (.parquet, .pqt), merging GCS-style shards
// (orders-0000001.pqt) into one collection per base name.
type Source struct{}

func (Source) Source(ctx context.Context, cfg handlers.Config, raw any) (coll.Stream, error) {
	opts := raw.(SrcOpts)
	if len(opts.Selections) == 0 {
		return nil, errs.New("parquet source requires at least one file")
	}

	type group struct {
		name  string
		files []FileSelection
	}
	byName := map[string][]FileSelection{}
	var order []string
	for _, sel := range opts.Selections {
		name := collectionName(sel.File)
		if _, ok := byName[name]; !ok {
			order = append(order, name)
		}
		byName[name] = append(byName[name], sel)
	}

	groups := make([]group, 0, len(order))
	for _, name := range order {
		files := append([]FileSelection(nil), byName[name]...)
		sort.Slice(files, func(i, j int) bool { return files[i].File < files[j].File })
		groups = append(groups, group{name: name, files: files})
	}

	return func(yield func(coll.Collection, error) bool) {
		for _, g := range groups {
			if err := ctx.Err(); err != nil {
				yield(coll.Collection{}, err)
				return
			}
			c := coll.Collection{
				Name: g.name,
				Rows: readSelections(ctx, g.files),
			}
			if !yield(c, nil) {
				return
			}
		}
	}, nil
}

func readSelections(ctx context.Context, files []FileSelection) coll.RowBatches {
	return func(yield func([]coll.Row, error) bool) {
		stopped := false
		guardedYield := func(batch []coll.Row, err error) bool {
			if stopped {
				return false
			}
			if !yield(batch, err) {
				stopped = true
				return false
			}
			return true
		}
		for _, sel := range files {
			if stopped {
				return
			}
			if err := readFile(ctx, sel, guardedYield); err != nil {
				guardedYield(nil, err)
				return
			}
		}
	}
}

func readFile(ctx context.Context, sel FileSelection, yield func([]coll.Row, error) bool) error {
	f, err := os.Open(sel.File)
	if err != nil {
		return errs.Wrap(err, "open parquet file", "path", sel.File)
	}
	defer f.Close()

	st, err := f.Stat()
	if err != nil {
		return errs.Wrap(err, "stat parquet file", "path", sel.File)
	}

	pf, err := parquet.OpenFile(f, st.Size())
	if err != nil {
		return errs.Wrap(err, "open parquet", "path", sel.File)
	}

	cols := parseColumns(sel.Columns)
	reader := parquet.NewGenericReader[any](pf)
	defer reader.Close()

	buf := make([]any, readBatchSize)
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, err := reader.Read(buf)
		if n > 0 {
			batch := make([]coll.Row, n)
			for i := 0; i < n; i++ {
				batch[i] = projectRow(anyToRow(buf[i]), cols)
			}
			if !yield(batch, nil) {
				return nil
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return errs.Wrap(err, "read parquet rows", "path", sel.File)
		}
	}
}
