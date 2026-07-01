package optparse_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/ceymard/swl-go/internal/optparse"
)

func TestGetHelpOneofNested(t *testing.T) {
	srcItem := optparse.Optparser(
		optparse.Arg("name").Required().Help("collection/table name"),
		optparse.Param("-q", "--query").As("query").Help("SQL query instead of SELECT *"),
	)
	p := optparse.Optparser(
		optparse.Param("-s", "--schema").As("schema").Default("public"),
		optparse.Arg("uri").Required(),
	).AddHandler(
		optparse.Oneof(srcItem).As("sources").Repeat(),
	)
	text := p.GetHelp("", "swl +pg [uri]")
	if !strings.Contains(text, "[sources...]") {
		t.Fatalf("missing sources placeholder:\n%s", text)
	}
	if !strings.Contains(text, "where sources: <options>") {
		t.Fatalf("missing nested section:\n%s", text)
	}
	if !strings.Contains(text, "-q") || !strings.Contains(text, "--query") {
		t.Fatalf("missing -q/--query in nested help:\n%s", text)
	}
}

func TestParseHelpFlag(t *testing.T) {
	p := optparse.Optparser(
		optparse.Flag("-t", "--truncate").As("truncate"),
		optparse.Arg("file").Required(),
	)
	_, err := p.Parse([]string{"x.db", "--help"})
	if !errors.Is(err, optparse.ErrHelp) {
		t.Fatalf("got err %v", err)
	}
	he, ok := err.(*optparse.HelpError)
	if !ok {
		t.Fatalf("got type %T", err)
	}
	text := he.Parser.GetHelp("", "swl sqlite")
	if !strings.Contains(text, "--truncate") || !strings.Contains(text, "[file]") {
		t.Fatalf("%q", text)
	}
}
