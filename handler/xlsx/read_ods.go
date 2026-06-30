package xlsx

import (
	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/errs"
	"github.com/knieriem/odf/ods"
)

type odsBook struct {
	file *ods.File
	doc  ods.Doc
}

func openODS(path string) (sheetReader, error) {
	f, err := ods.Open(path)
	if err != nil {
		return nil, errs.Wrap(err, "open ods", "path", path)
	}
	b := &odsBook{file: f}
	if err := f.ParseContent(&b.doc); err != nil {
		f.Close()
		return nil, errs.Wrap(err, "parse ods content", "path", path)
	}
	return b, nil
}

func (b *odsBook) Close() error {
	if b.file == nil {
		return nil
	}
	return b.file.Close()
}

func (b *odsBook) sheetSpecs(opts SrcOpts) ([]SheetSpec, error) {
	names := make([]string, 0, len(b.doc.Table))
	for _, t := range b.doc.Table {
		names = append(names, t.Name)
	}
	return resolveSheetSpecs(names, opts), nil
}

func (b *odsBook) readSheet(spec SheetSpec) ([]coll.Row, error) {
	var table *ods.Table
	for i := range b.doc.Table {
		if b.doc.Table[i].Name == spec.Name {
			table = &b.doc.Table[i]
			break
		}
	}
	if table == nil {
		return nil, errs.New(`no such sheet "` + spec.Name + `"`)
	}
	return rowsFromTable(table.Strings(), spec, columnName)
}

var _ sheetReader = (*odsBook)(nil)
