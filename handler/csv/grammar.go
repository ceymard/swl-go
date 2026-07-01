package csv

import (
	"github.com/ceymard/swl-go/internal/cli"
	"github.com/ceymard/swl-go/internal/optparse"
)

// SrcOpts is parsed argv for csv-src.
type SrcOpts struct {
	Files           []string
	Delimiter       string
	Gunzip          bool
	Quote           rune
	NoEmpty         bool
	EmptyIsNull     bool
	Numbers         bool
	Escape          rune
	Encoding        string
	Headers         []string
	Collection      *string
	Merge           map[string]any
	SimplifyHeaders bool
	cli.BaseOpts
}

// SinkOpts is parsed argv for csv-sink.
type SinkOpts struct {
	Path      string
	Delimiter rune
	Quote     rune
	Charset   string
	NoHeaders bool
	cli.BaseOpts
}

var srcParser = optparse.Optparser(
	optparse.Param("-d", "--delimiter").As("delimiter").Default(",").Help("Field delimiter"),
	optparse.Flag("--gunzip").As("gunzip").Help("Decompress .gz files"),
	optparse.Param("-q", "--quote").As("quote").Help("Quote character (default \")"),
	optparse.Flag("-n", "--no-empty").As("noempty").Help("Skip rows where all fields are empty"),
	optparse.Flag("-N", "--empty-null").As("emptyisnull").Help("Treat empty fields as null"),
	optparse.Flag("-u").As("numbers").Help("Parse numeric fields"),
	optparse.Param("-e", "--escape").As("escape").Help("Escape character"),
	optparse.Param("-E", "--encoding").As("encoding").Default("utf-8").Help("Character encoding"),
	optparse.Param("-h", "--headers").As("headers").Default("").Help("Comma-separated header names"),
	optparse.Param("-c", "--collection").As("collection").Help("Collection name (default: filename stem)"),
	optparse.Param("-m", "--merge").As("merge").Help("Add null columns (comma-separated names)"),
	optparse.Flag("-s", "--simplify-headers").As("simplify_headers").Help("Normalize header names"),
	optparse.Arg("files").Required().Repeat().Help("One or more CSV file paths"),
)

var sinkParser = optparse.Optparser(
	optparse.Arg("path").Required().Help("Output file, directory, or % pattern"),
	optparse.Param("-d", "--delimiter").As("delimiter").Default(";").Help("Field delimiter"),
	optparse.Param("-q", "--quote").As("quote").Default(`"`).Help("Quote character"),
	optparse.Param("--charset").As("charset").Default("utf-8").Help("Character encoding"),
	optparse.Flag("-n", "--no-headers").As("no_headers").Help("Omit header row"),
).Include(optparse.DefaultOpts)

// SrcOptParser returns the optparse parser for csv-src.
func SrcOptParser() *optparse.Parser { return srcParser }

// SinkOptParser returns the optparse parser for csv-sink.
func SinkOptParser() *optparse.Parser { return sinkParser }

// ParseSrcOptions parses csv-src argv; target is the first file path.
func ParseSrcOptions(target string, tail []string) (any, error) {
	m, err := srcParser.Parse(append([]string{target}, tail...))
	if err != nil {
		return nil, err
	}
	files := optparse.StringSlice(m, "files")
	if len(files) == 0 {
		files = []string{target}
	}
	opts := SrcOpts{
		Files:           files,
		Delimiter:       optparse.Str(m, "delimiter"),
		Gunzip:          optparse.Bool(m, "gunzip"),
		NoEmpty:         optparse.Bool(m, "noempty"),
		EmptyIsNull:     optparse.Bool(m, "emptyisnull"),
		Numbers:         optparse.Bool(m, "numbers"),
		SimplifyHeaders: optparse.Bool(m, "simplify_headers"),
		Encoding:        optparse.Str(m, "encoding"),
		Collection:      optparse.StrPtr(m, "collection"),
	}
	if q := optparse.Str(m, "quote"); q != "" {
		opts.Quote = rune(q[0])
	} else {
		opts.Quote = '"'
	}
	if e := optparse.Str(m, "escape"); e != "" {
		opts.Escape = rune(e[0])
	}
	if h := optparse.Str(m, "headers"); h != "" {
		opts.Headers = splitList(h)
	}
	if merge := optparse.Str(m, "merge"); merge != "" {
		opts.Merge = mergeColumns(merge)
	}
	return opts, nil
}

// ParseSinkOptions parses csv-sink argv; target is the output path.
func ParseSinkOptions(target string, tail []string) (any, error) {
	m, err := sinkParser.Parse(append([]string{target}, tail...))
	if err != nil {
		return nil, err
	}
	opts := SinkOpts{
		Path:      optparse.Str(m, "path"),
		Charset:   optparse.Str(m, "charset"),
		NoHeaders: optparse.Bool(m, "no_headers"),
		BaseOpts:  cli.BaseOptsFrom(m),
	}
	if d := optparse.Str(m, "delimiter"); d != "" {
		opts.Delimiter = rune(d[0])
	} else {
		opts.Delimiter = ';'
	}
	if q := optparse.Str(m, "quote"); q != "" {
		opts.Quote = rune(q[0])
	} else {
		opts.Quote = '"'
	}
	return opts, nil
}
