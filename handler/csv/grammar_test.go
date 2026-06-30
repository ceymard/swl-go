package csv

import "testing"

func TestParseSinkOptionsDelimiter(t *testing.T) {
	o, err := ParseSinkOptions("/tmp/out.csv", []string{"-d=,"})
	if err != nil {
		t.Fatal(err)
	}
	opts := o.(SinkOpts)
	if opts.Delimiter != ',' {
		t.Fatalf("delimiter %q", string(opts.Delimiter))
	}
}

func TestParseSrcOptionsNumbers(t *testing.T) {
	o, err := ParseSrcOptions("f.csv", []string{"-u"})
	if err != nil {
		t.Fatal(err)
	}
	opts := o.(SrcOpts)
	if !opts.Numbers {
		t.Fatal("expected -u")
	}
}
