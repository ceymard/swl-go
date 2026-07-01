package json

import (
	"github.com/ceymard/swl-go/internal/cli"
	"github.com/ceymard/swl-go/internal/optparse"
)

// SrcOpts is the parsed argv for json-src.
type SrcOpts struct {
	File       string
	Encoding   *string
	Collection *string
	cli.BaseOpts
}

var srcParser = optparse.Optparser(
	optparse.Param("-e", "--encoding").As("encoding").Help("Character encoding (default utf-8)"),
	optparse.Param("-c", "--collection").As("collection").Help("Collection name for array/object roots or inline JSON"),
	optparse.Arg("file").Required().Help("JSON file path, or inline JSON starting with [ or {"),
).Include(optparse.DefaultOpts)

// SrcOptParser returns the optparse parser for json-src.
func SrcOptParser() *optparse.Parser { return srcParser }

// SinkOptParser returns the optparse parser for json-sink.
func SinkOptParser() *optparse.Parser { return sinkParser }

// ParseSrcOptions parses json-src flags; target is the file path or inline JSON.
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

// SinkOpts is the parsed argv for json-sink.
type SinkOpts struct {
	Path   string
	Object bool
	cli.BaseOpts
}

var sinkParser = optparse.Optparser(
	optparse.Flag("-o", "--object").As("object").Help("Write one JSON object keyed by collection name"),
	optparse.Arg("path").Required().Help("Output file, directory, % pattern, or empty for cwd"),
).Include(optparse.DefaultOpts)

// ParseSinkOptions parses json-sink flags; target is the output path.
func ParseSinkOptions(target string, tail []string) (any, error) {
	m, err := sinkParser.Parse(append([]string{target}, tail...))
	if err != nil {
		return nil, err
	}
	return SinkOpts{
		Path:     optparse.Str(m, "path"),
		Object:   optparse.Bool(m, "object"),
		BaseOpts: cli.BaseOptsFrom(m),
	}, nil
}
