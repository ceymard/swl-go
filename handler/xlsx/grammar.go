package xlsx

import (
	"github.com/ceymard/swl-go/internal/cli"
	"github.com/ceymard/swl-go/internal/optparse"
)

// SheetSpec selects one worksheet/collection from a spreadsheet file.
type SheetSpec struct {
	Name         string
	Rename       string
	IgnoreErrors bool
	Include      bool
}

// SrcOpts is parsed argv for xlsx-src.
type SrcOpts struct {
	File         string
	IgnoreErrors bool
	Include      bool
	Sheets       []SheetSpec
	cli.BaseOpts
}

var sheetFlags = optparse.Optparser(
	optparse.Param("-r", "--rename").As("rename").Help("Rename this collection"),
	optparse.Flag("-e", "--ignore-errors").As("ignore_errors").Help("Ignore formula errors"),
	optparse.Flag("-i", "--include").As("include").Help("Include columns starting with '.'"),
)

var sheetParser = optparse.Optparser(
	optparse.Arg("name").Required(),
).Include(sheetFlags)

// SinkOpts is parsed argv for xlsx-sink.
type SinkOpts struct {
	File       string
	Uncompress bool
	cli.BaseOpts
}

var sinkParser = optparse.Optparser(
	optparse.Arg("file").Required(),
	optparse.Flag("-u", "--uncompress").As("uncompress").Help("Disable XLSX compression (ignored; excelize always compresses)"),
).Include(optparse.DefaultOpts)

// SinkOptParser returns the optparse parser for xlsx-sink.
func SinkOptParser() *optparse.Parser { return sinkParser }

// ParseSinkOptions parses xlsx-sink flags; target is the output file path.
func ParseSinkOptions(target string, tail []string) (any, error) {
	m, err := sinkParser.Parse(append([]string{target}, tail...))
	if err != nil {
		return nil, err
	}
	return SinkOpts{
		File:       optparse.Str(m, "file"),
		Uncompress: optparse.Bool(m, "uncompress"),
		BaseOpts:   cli.BaseOptsFrom(m),
	}, nil
}

var srcParser = optparse.Optparser(
	optparse.Arg("file").Required(),
).Include(sheetFlags).Include(optparse.DefaultOpts).AddHandler(
	optparse.Oneof(sheetParser).As("collections").Repeat().Help("Select sheets by name"),
)

// SrcOptParser returns the optparse parser for xlsx-src (help text).
func SrcOptParser() *optparse.Parser { return srcParser }

// ParseSrcOptions parses xlsx-src flags; target is the spreadsheet file path.
func ParseSrcOptions(target string, tail []string) (any, error) {
	m, err := srcParser.Parse(append([]string{target}, tail...))
	if err != nil {
		return nil, err
	}
	opts := SrcOpts{
		File:         optparse.Str(m, "file"),
		IgnoreErrors: optparse.Bool(m, "ignore_errors"),
		Include:      optparse.Bool(m, "include"),
		BaseOpts:     cli.BaseOptsFrom(m),
	}
	for _, col := range optparse.MapSlice(m, "collections") {
		spec := SheetSpec{
			Name:         optparse.Str(col, "name"),
			Rename:       optparse.Str(col, "rename"),
			IgnoreErrors: optparse.Bool(col, "ignore_errors"),
			Include:      optparse.Bool(col, "include"),
		}
		opts.Sheets = append(opts.Sheets, spec)
	}
	return opts, nil
}
