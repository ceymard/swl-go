package optparse_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/ceymard/swl-go/internal/optparse"
)

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
