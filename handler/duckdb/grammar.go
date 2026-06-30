package duckdb

import (
	"github.com/ceymard/swl-go/internal/cli"
	"github.com/ceymard/swl-go/internal/optparse"
)

// TableSpec selects one collection from a DuckDB source.
type TableSpec struct {
	Name  string
	Query string
}

// SrcOpts is parsed argv for duckdb-src.
type SrcOpts struct {
	File   string
	Tables []TableSpec
	cli.BaseOpts
}

var srcParser = optparse.Optparser(
	optparse.Arg("file").Required(),
).Include(optparse.DefaultOpts).AddHandler(
	optparse.Oneof(optparse.DefaultColSQLOpts).As("collections").Repeat(),
)

// SrcOptParser returns the optparse parser for duckdb-src.
func SrcOptParser() *optparse.Parser { return srcParser }

// SinkOptParser returns the optparse parser for duckdb-sink.
func SinkOptParser() *optparse.Parser { return sinkParser }

// ParseSrcOptions parses duckdb-src flags; target is the database file path.
func ParseSrcOptions(target string, tail []string) (any, error) {
	m, err := srcParser.Parse(append([]string{target}, tail...))
	if err != nil {
		return nil, err
	}
	opts := SrcOpts{
		File:     optparse.Str(m, "file"),
		BaseOpts: cli.BaseOptsFrom(m),
	}
	for _, col := range optparse.MapSlice(m, "collections") {
		spec := TableSpec{Name: optparse.Str(col, "name")}
		if q := optparse.Str(col, "query"); q != "" {
			spec.Query = q
		}
		opts.Tables = append(opts.Tables, spec)
	}
	return opts, nil
}

// SinkOpts is parsed argv for duckdb-sink.
type SinkOpts struct {
	File     string
	Truncate bool
	Drop     bool
	Upsert   bool
	cli.BaseOpts
}

var colSinkOpts = optparse.Optparser(
	optparse.Flag("-t", "--truncate").As("truncate"),
	optparse.Flag("-d", "--drop").As("drop"),
	optparse.Flag("-u", "--upsert").As("upsert"),
)

var colSinkParser = optparse.Optparser(
	optparse.Arg("name").Required(),
).Include(colSinkOpts)

var sinkParser = optparse.Optparser(
	optparse.Arg("file").Required(),
).Include(optparse.DefaultOpts).Include(colSinkOpts).AddHandler(
	optparse.Oneof(colSinkParser).As("collections").Repeat(),
)

// ParseSinkOptions parses duckdb-sink flags; target is the database file path.
func ParseSinkOptions(target string, tail []string) (any, error) {
	m, err := sinkParser.Parse(append([]string{target}, tail...))
	if err != nil {
		return nil, err
	}
	opts := SinkOpts{
		File:     optparse.Str(m, "file"),
		Truncate: optparse.Bool(m, "truncate"),
		Drop:     optparse.Bool(m, "drop"),
		Upsert:   optparse.Bool(m, "upsert"),
		BaseOpts: cli.BaseOptsFrom(m),
	}
	for _, col := range optparse.MapSlice(m, "collections") {
		if optparse.Bool(col, "truncate") {
			opts.Truncate = true
		}
		if optparse.Bool(col, "drop") {
			opts.Drop = true
		}
		if optparse.Bool(col, "upsert") {
			opts.Upsert = true
		}
	}
	return opts, nil
}
