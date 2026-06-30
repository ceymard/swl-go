package json

import (
	"github.com/ceymard/swl-go/internal/cli"
)

// SrcOpts is the parsed argv for json-src.
type SrcOpts struct {
	File       string // path or inline JSON (from pipeline target)
	Encoding   *string
	Collection *string
	cli.BaseOpts
}

type srcParser struct {
	Encoding   *string `parser:"( ( '-e' | '--encoding' ) @Arg )?"`
	Collection *string `parser:"( ( '-c' | '--collection' ) @Arg )?"`
	cli.BaseOpts
}

// ParseSrcOptions parses json-src flags; target is the file path or inline JSON.
func ParseSrcOptions(target string, tail []string) (any, error) {
	p, err := cli.BuildParser[srcParser]()
	if err != nil {
		return nil, err
	}
	o, err := cli.ParseArgs(p, tail)
	if err != nil {
		return nil, err
	}
	return SrcOpts{
		File:       target,
		Encoding:   o.Encoding,
		Collection: o.Collection,
		BaseOpts:   o.BaseOpts,
	}, nil
}

// SinkOpts is the parsed argv for json-sink.
type SinkOpts struct {
	Path   string // output path (empty → cwd at run time)
	Object bool   // -o: wrap collections in a JSON object
	cli.BaseOpts
}

type sinkParser struct {
	Object bool `parser:"( '-o' | '--object' )?"`
	cli.BaseOpts
}

// ParseSinkOptions parses json-sink flags; target is the output path.
func ParseSinkOptions(target string, tail []string) (any, error) {
	p, err := cli.BuildParser[sinkParser]()
	if err != nil {
		return nil, err
	}
	o, err := cli.ParseArgs(p, tail)
	if err != nil {
		return nil, err
	}
	return SinkOpts{
		Path:     target,
		Object:   o.Object,
		BaseOpts: o.BaseOpts,
	}, nil
}
