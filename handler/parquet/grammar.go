package parquet

import (
	"github.com/ceymard/swl-go/internal/cli"
	"github.com/ceymard/swl-go/internal/optparse"
)

// FileSelection is one parquet file with optional column projection.
type FileSelection struct {
	File    string
	Columns string
}

// SrcOpts is parsed argv for parquet-src.
type SrcOpts struct {
	Selections []FileSelection
	cli.BaseOpts
}

// SinkOpts is parsed argv for parquet-sink.
type SinkOpts struct {
	Path string
	cli.BaseOpts
}

var fileSelectionParser = optparse.Optparser(
	optparse.Arg("file").Required(),
	optparse.Param("-c", "--columns").As("columns").Help("Comma-separated columns to read"),
)

var srcParser = optparse.Optparser(
	optparse.Oneof(fileSelectionParser).As("selections").Repeat(),
).Include(optparse.DefaultOpts)

var sinkParser = optparse.Optparser(
	optparse.Arg("path").Required(),
).Include(optparse.DefaultOpts)

// SrcOptParser returns the optparse parser for parquet-src.
func SrcOptParser() *optparse.Parser { return srcParser }

// SinkOptParser returns the optparse parser for parquet-sink.
func SinkOptParser() *optparse.Parser { return sinkParser }

// ParseSrcOptions parses parquet-src flags; target is the first file path.
func ParseSrcOptions(target string, tail []string) (any, error) {
	m, err := srcParser.Parse(append([]string{target}, tail...))
	if err != nil {
		return nil, err
	}
	opts := SrcOpts{BaseOpts: cli.BaseOptsFrom(m)}
	for _, sel := range optparse.MapSlice(m, "selections") {
		opts.Selections = append(opts.Selections, FileSelection{
			File:    optparse.Str(sel, "file"),
			Columns: optparse.Str(sel, "columns"),
		})
	}
	if len(opts.Selections) == 0 && target != "" {
		opts.Selections = []FileSelection{{File: target}}
	}
	return opts, nil
}

// ParseSinkOptions parses parquet-sink flags; target is the output path.
func ParseSinkOptions(target string, tail []string) (any, error) {
	m, err := sinkParser.Parse(append([]string{target}, tail...))
	if err != nil {
		return nil, err
	}
	return SinkOpts{
		Path:     optparse.Str(m, "path"),
		BaseOpts: cli.BaseOptsFrom(m),
	}, nil
}
