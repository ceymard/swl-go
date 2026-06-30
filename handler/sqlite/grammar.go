package sqlite

import (
	"github.com/ceymard/swl-go/internal/cli"
)

// TableSpec selects one collection from a SQL source.
type TableSpec struct {
	Name  string
	Query string // empty → SELECT * FROM name
}

// SrcOpts is parsed argv for sqlite-src.
type SrcOpts struct {
	File   string
	Tables []TableSpec
	cli.BaseOpts
}

type tableSpecParser struct {
	Name  string  `@Arg`
	Query *string `parser:"( ( '-q' | '--query' ) @Arg )?"`
}

type srcTailParser struct {
	Tables []tableSpecParser `@@*`
}

// ParseSrcOptions parses sqlite-src flags; target is the database file path.
func ParseSrcOptions(target string, tail []string) (any, error) {
	p, err := cli.BuildParser[srcTailParser]()
	if err != nil {
		return nil, err
	}
	o, err := cli.ParseArgs(p, tail)
	if err != nil {
		return nil, err
	}
	opts := SrcOpts{File: target, BaseOpts: cli.BaseOpts{}}
	for _, t := range o.Tables {
		spec := TableSpec{Name: t.Name}
		if t.Query != nil {
			spec.Query = *t.Query
		}
		opts.Tables = append(opts.Tables, spec)
	}
	return opts, nil
}

// SinkOpts is parsed argv for sqlite-sink.
type SinkOpts struct {
	File     string
	Truncate bool
	Drop     bool
	Upsert   bool
	cli.BaseOpts
}

type sinkParser struct {
	Truncate bool `parser:"( '-t' | '--truncate' )?"`
	Drop     bool `parser:"( '-d' | '--drop' )?"`
	Upsert   bool `parser:"( '-u' | '--upsert' )?"`
	cli.BaseOpts
}

// ParseSinkOptions parses sqlite-sink flags; target is the database file path.
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
		File:     target,
		Truncate: o.Truncate,
		Drop:     o.Drop,
		Upsert:   o.Upsert,
		BaseOpts: o.BaseOpts,
	}, nil
}
