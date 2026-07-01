package mssql

import (
	"github.com/ceymard/swl-go/internal/cli"
	"github.com/ceymard/swl-go/internal/optparse"
)

// TableSpec selects one collection from an MSSQL source.
type TableSpec struct {
	Name  string
	Query string
}

// SrcOpts is parsed argv for mssql-src.
type SrcOpts struct {
	URI     string
	Sources []TableSpec
	cli.BaseOpts
}

var srcItemParser = optparse.DefaultColSQLOpts

var srcParser = optparse.Optparser(
	optparse.Arg("uri").Required().Help("SQL Server URI or ADO connection string"),
).Include(optparse.DefaultOpts).AddHandler(
	optparse.Oneof(srcItemParser).As("sources").Repeat().Help("Schema.table or query to emit as a collection"),
)

// SrcOptParser returns the optparse parser for mssql-src.
func SrcOptParser() *optparse.Parser { return srcParser }

// SinkOptParser returns the optparse parser for mssql-sink.
func SinkOptParser() *optparse.Parser { return sinkParser }

// ParseSrcOptions parses mssql-src flags; target is the connection URI.
func ParseSrcOptions(target string, tail []string) (any, error) {
	m, err := srcParser.Parse(append([]string{target}, tail...))
	if err != nil {
		return nil, err
	}
	opts := SrcOpts{
		URI:      optparse.Str(m, "uri"),
		BaseOpts: cli.BaseOptsFrom(m),
	}
	for _, src := range optparse.MapSlice(m, "sources") {
		spec := TableSpec{Name: optparse.Str(src, "name")}
		if q := optparse.Str(src, "query"); q != "" {
			spec.Query = q
		}
		opts.Sources = append(opts.Sources, spec)
	}
	return opts, nil
}

// SinkOpts is parsed argv for mssql-sink.
type SinkOpts struct {
	URI        string
	Truncate   bool
	Drop       bool
	Upsert     bool
	AutoCreate bool
	cli.BaseOpts
}

var colSinkParser = optparse.Optparser(
	optparse.Arg("name").Required().Help("Collection name (schema.table)"),
).Include(optparse.DefaultColSinkOptsAuto)

var sinkParser = optparse.Optparser(
	optparse.Arg("uri").Required().Help("SQL Server URI or ADO connection string"),
).Include(optparse.DefaultOpts).Include(optparse.DefaultColSinkOptsAuto).AddHandler(
	optparse.Oneof(colSinkParser).As("collections").Repeat().Help("Per-collection sink options"),
)

// ParseSinkOptions parses mssql-sink flags; target is the connection URI.
func ParseSinkOptions(target string, tail []string) (any, error) {
	m, err := sinkParser.Parse(append([]string{target}, tail...))
	if err != nil {
		return nil, err
	}
	opts := SinkOpts{
		URI:        optparse.Str(m, "uri"),
		Truncate:   optparse.Bool(m, "truncate"),
		Drop:       optparse.Bool(m, "drop"),
		Upsert:     optparse.Bool(m, "upsert"),
		AutoCreate: optparse.Bool(m, "auto_create"),
		BaseOpts:   cli.BaseOptsFrom(m),
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
		if optparse.Bool(col, "auto_create") {
			opts.AutoCreate = true
		}
	}
	return opts, nil
}
