package optparse_test

import (
	"testing"

	"github.com/ceymard/swl-go/internal/optparse"
)

func TestExpandFlags(t *testing.T) {
	got := optparse.ExpandFlags([]string{"-xu", "--foo=bar", "file", "-d", ","})
	want := []string{"-x", "-u", "--foo", "bar", "file", "-d", ","}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestCSVSrcFlags(t *testing.T) {
	p := optparse.Optparser(
		optparse.Param("-d", "--delimiter").As("delimiter").Default(","),
		optparse.Flag("-u").As("numbers"),
		optparse.Arg("files").Required().Repeat(),
	)
	m, err := p.Parse([]string{"a.csv", "-d", ",", "-u"})
	if err != nil {
		t.Fatal(err)
	}
	if optparse.Str(m, "delimiter") != "," {
		t.Fatalf("delimiter %q", optparse.Str(m, "delimiter"))
	}
	if !optparse.Bool(m, "numbers") {
		t.Fatal("expected numbers")
	}
	files := optparse.StringSlice(m, "files")
	if len(files) != 1 || files[0] != "a.csv" {
		t.Fatalf("files %v", files)
	}
}

func TestSQLiteSinkUpsert(t *testing.T) {
	colOpts := optparse.Optparser(
		optparse.Flag("-t", "--truncate").As("truncate"),
		optparse.Flag("-d", "--drop").As("drop"),
		optparse.Flag("-u", "--upsert").As("upsert"),
	)
	colParser := optparse.Optparser(
		optparse.Arg("name").Required(),
	).Include(colOpts)
	p := optparse.Optparser(
		optparse.Arg("file").Required(),
	).Include(optparse.DefaultOpts).Include(colOpts).AddHandler(
		optparse.Oneof(colParser).As("collections").Repeat(),
	)
	m, err := p.Parse([]string{"out.db", "-u", "users"})
	if err != nil {
		t.Fatal(err)
	}
	if !optparse.Bool(m, "upsert") {
		t.Fatal("expected upsert")
	}
	cols := optparse.MapSlice(m, "collections")
	if len(cols) != 1 || optparse.Str(cols[0], "name") != "users" {
		t.Fatalf("collections %v", cols)
	}
}
