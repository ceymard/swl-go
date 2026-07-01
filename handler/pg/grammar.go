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

var srcItemParser = optparse.DefaultColSQLOpts

var srcParser = optparse.Optparser(
	optparse.Param("-s", "--schema").As("schema").Default("public").Help("Default schema when no sources are listed"),
	optparse.Arg("uri").Required().Help("Postgres connection URI (postgres://user:pass@host/db)"),
).Include(optparse.DefaultOpts).AddHandler(
	optparse.Oneof(srcItemParser).As("sources").Repeat().Help("Table or query to emit as a collection"),
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
	optparse.Flag("-n", "--table-name").As("table_name").Help("Use a different table name than the collection"),
	optparse.Flag("-a", "--auto-create").As("auto_create").Help("Create table if it does not exist"),
	optparse.Flag("-t", "--truncate").As("truncate").Help("Truncate table before load"),
	optparse.Flag("-d", "--drop").As("drop").Help("Drop table before load"),
	optparse.Flag("-u", "--upsert").As("upsert").Help("Upsert rows on conflict"),
	optparse.Flag("--do-nothing").As("do_nothing").Help("ON CONFLICT DO NOTHING"),
	optparse.Flag("-U", "--update").As("update").Help("Update existing rows on conflict"),
	optparse.Flag("-i", "--drop-indexes").As("drop_indexes").Help("Drop indexes during load and recreate after"),
)

var colSinkParser = optparse.Optparser(
	optparse.Arg("name").Required().Help("Collection/table name"),
).Include(colSinkOptsParser)

var sinkParser = optparse.Optparser(
	optparse.Arg("uri").Required().Help("Postgres connection URI"),
	optparse.Flag("--disable-triggers").As("disable_triggers").Help("Disable triggers before load"),
	optparse.Flag("-n", "--notice").As("notice").Help("Show NOTICE messages"),
	optparse.Flag("-y", "--notify").As("notify").Help("Show LISTEN/NOTIFY traffic"),
	optparse.Param("-s", "--schema").As("schema").Default("public").Help("Default schema for collections"),
	optparse.Flag("--ignore-non-existing").As("ignore_nonexisting").Help("Skip collections whose table does not exist"),
).Include(optparse.DefaultOpts).Include(colSinkOptsParser).AddHandler(
	optparse.Oneof(colSinkParser).As("collections").Repeat().Help("Per-collection sink options"),
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
