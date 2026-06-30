package pg

import (
	"github.com/ceymard/swl-go/internal/cli"
	"github.com/ceymard/swl-go/internal/optparse"
)

// TableSpec selects one collection from a PG source.
type TableSpec struct {
	Name  string
	Query string
}

// SrcOpts is parsed argv for pg-src.
type SrcOpts struct {
	URI     string
	Schema  string
	Sources []TableSpec
	cli.BaseOpts
}

var srcItemParser = optparse.Optparser(
	optparse.Arg("name").Required(),
	optparse.Param("-q", "--query").As("query"),
)

var srcParser = optparse.Optparser(
	optparse.Param("-s", "--schema").As("schema").Default("public"),
	optparse.Arg("uri").Required(),
).Include(optparse.DefaultOpts).AddHandler(
	optparse.Oneof(srcItemParser).As("sources").Repeat(),
)

// SrcOptParser returns the optparse parser for pg-src.
func SrcOptParser() *optparse.Parser { return srcParser }

// SinkOptParser returns the optparse parser for pg-sink.
func SinkOptParser() *optparse.Parser { return sinkParser }

// ParseSrcOptions parses pg-src flags; target is the postgres URI.
func ParseSrcOptions(target string, tail []string) (any, error) {
	m, err := srcParser.Parse(append([]string{target}, tail...))
	if err != nil {
		return nil, err
	}
	opts := SrcOpts{
		URI:      optparse.Str(m, "uri"),
		Schema:   optparse.Str(m, "schema"),
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

// SinkOpts is parsed argv for pg-sink.
type SinkOpts struct {
	URI               string
	Schema            string
	AutoCreate        bool
	Truncate          bool
	Drop              bool
	Upsert            bool
	DoNothing         bool
	Update            bool
	DisableTriggers   bool
	Notice            bool
	IgnoreNonExisting bool
	DropIndexes       bool
	Collections       map[string]colSinkOpts
	cli.BaseOpts
}

type colSinkOpts struct {
	TableName    bool
	AutoCreate   bool
	Truncate     bool
	Drop         bool
	Upsert       bool
	DoNothing    bool
	Update       bool
	DropIndexes  bool
}

var colSinkOptsParser = optparse.Optparser(
	optparse.Flag("-n", "--table-name").As("table_name"),
	optparse.Flag("-a", "--auto-create").As("auto_create"),
	optparse.Flag("-t", "--truncate").As("truncate"),
	optparse.Flag("-d", "--drop").As("drop"),
	optparse.Flag("-u", "--upsert").As("upsert"),
	optparse.Flag("--do-nothing").As("do_nothing"),
	optparse.Flag("-U", "--update").As("update"),
	optparse.Flag("-i", "--drop-indexes").As("drop_indexes"),
)

var colSinkParser = optparse.Optparser(
	optparse.Arg("name").Required(),
).Include(colSinkOptsParser)

var sinkParser = optparse.Optparser(
	optparse.Arg("uri").Required(),
	optparse.Flag("--disable-triggers").As("disable_triggers"),
	optparse.Flag("-n", "--notice").As("notice"),
	optparse.Flag("-y", "--notify").As("notify"),
	optparse.Param("-s", "--schema").As("schema").Default("public"),
	optparse.Flag("--ignore-non-existing").As("ignore_nonexisting"),
).Include(optparse.DefaultOpts).Include(colSinkOptsParser).AddHandler(
	optparse.Oneof(colSinkParser).As("collections").Repeat(),
)

// ParseSinkOptions parses pg-sink flags; target is the postgres URI.
func ParseSinkOptions(target string, tail []string) (any, error) {
	m, err := sinkParser.Parse(append([]string{target}, tail...))
	if err != nil {
		return nil, err
	}
	opts := SinkOpts{
		URI:               optparse.Str(m, "uri"),
		Schema:            optparse.Str(m, "schema"),
		AutoCreate:        optparse.Bool(m, "auto_create"),
		Truncate:          optparse.Bool(m, "truncate"),
		Drop:              optparse.Bool(m, "drop"),
		Upsert:            optparse.Bool(m, "upsert"),
		DoNothing:         optparse.Bool(m, "do_nothing"),
		Update:            optparse.Bool(m, "update"),
		DisableTriggers:   optparse.Bool(m, "disable_triggers"),
		Notice:            optparse.Bool(m, "notice"),
		IgnoreNonExisting: optparse.Bool(m, "ignore_nonexisting"),
		DropIndexes:       optparse.Bool(m, "drop_indexes"),
		BaseOpts:          cli.BaseOptsFrom(m),
		Collections:       map[string]colSinkOpts{},
	}
	for _, col := range optparse.MapSlice(m, "collections") {
		name := optparse.Str(col, "name")
		c := colSinkOpts{
			TableName:   optparse.Bool(col, "table_name"),
			AutoCreate:  optparse.Bool(col, "auto_create"),
			Truncate:    optparse.Bool(col, "truncate"),
			Drop:        optparse.Bool(col, "drop"),
			Upsert:      optparse.Bool(col, "upsert"),
			DoNothing:   optparse.Bool(col, "do_nothing"),
			Update:      optparse.Bool(col, "update"),
			DropIndexes: optparse.Bool(col, "drop_indexes"),
		}
		if opts.Truncate {
			c.Truncate = true
		}
		if opts.Drop {
			c.Drop = true
		}
		if opts.Upsert {
			c.Upsert = true
		}
		if opts.Update {
			c.Update = true
		}
		if opts.AutoCreate {
			c.AutoCreate = true
		}
		opts.Collections[name] = c
	}
	return opts, nil
}
