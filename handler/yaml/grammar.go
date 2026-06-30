package yaml

import (
	"github.com/ceymard/swl-go/internal/cli"
	"github.com/ceymard/swl-go/internal/optparse"
)

// SrcOpts is parsed argv for yaml-src.
type SrcOpts struct {
	File       string
	Encoding   *string
	Collection *string
	cli.BaseOpts
}

var srcParser = optparse.Optparser(
	optparse.Param("-e", "--encoding").As("encoding"),
	optparse.Param("-c", "--collection").As("collection"),
	optparse.Arg("file").Required(),
).Include(optparse.DefaultOpts)

// SrcOptParser returns the optparse parser for yaml-src.
func SrcOptParser() *optparse.Parser { return srcParser }

// SinkOptParser returns the optparse parser for yaml-sink.
func SinkOptParser() *optparse.Parser { return sinkParser }

// ParseSrcOptions parses yaml-src flags; target is the file path.
func ParseSrcOptions(target string, tail []string) (any, error) {
	m, err := srcParser.Parse(append([]string{target}, tail...))
	if err != nil {
		return nil, err
	}
	return SrcOpts{
		File:       optparse.Str(m, "file"),
		Encoding:   optparse.StrPtr(m, "encoding"),
		Collection: optparse.StrPtr(m, "collection"),
		BaseOpts:   cli.BaseOptsFrom(m),
	}, nil
}

// SinkOpts is parsed argv for yaml-sink.
type SinkOpts struct {
	Path string
	cli.BaseOpts
}

var sinkParser = optparse.Optparser(
	optparse.Arg("path").Required(),
).Include(optparse.DefaultOpts)

// ParseSinkOptions parses yaml-sink flags; target is the output path.
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
