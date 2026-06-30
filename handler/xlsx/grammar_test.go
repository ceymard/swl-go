package xlsx_test

import (
	"testing"

	"github.com/ceymard/swl-go/handler/xlsx"
)

func TestParseSrcOptionsFileOnly(t *testing.T) {
	opts, err := xlsx.ParseSrcOptions("book.xlsx", nil)
	if err != nil {
		t.Fatal(err)
	}
	o := opts.(xlsx.SrcOpts)
	if o.File != "book.xlsx" || len(o.Sheets) != 0 {
		t.Fatalf("opts %+v", o)
	}
}

func TestParseSrcOptionsSheetSelection(t *testing.T) {
	opts, err := xlsx.ParseSrcOptions("book.xlsx", []string{"Sheet1", "-r", "people", "-i"})
	if err != nil {
		t.Fatal(err)
	}
	o := opts.(xlsx.SrcOpts)
	if len(o.Sheets) != 1 {
		t.Fatalf("sheets %d", len(o.Sheets))
	}
	if o.Sheets[0].Name != "Sheet1" || o.Sheets[0].Rename != "people" || !o.Sheets[0].Include {
		t.Fatalf("sheet %+v", o.Sheets[0])
	}
}

func TestParseSrcOptionsGlobalFlags(t *testing.T) {
	opts, err := xlsx.ParseSrcOptions("book.xlsx", []string{"-e", "-i"})
	if err != nil {
		t.Fatal(err)
	}
	o := opts.(xlsx.SrcOpts)
	if !o.IgnoreErrors || !o.Include {
		t.Fatalf("opts %+v", o)
	}
}

func TestSrcOptParserHelp(t *testing.T) {
	text := xlsx.SrcOptParser().GetHelp("", "swl +xlsx")
	if text == "" {
		t.Fatal("empty help")
	}
}
