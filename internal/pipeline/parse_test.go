package pipeline_test

import (
	"testing"

	"github.com/ceymard/swl-go/handler/json"
	"github.com/ceymard/swl-go/internal/pipeline"
	"github.com/ceymard/swl-go/internal/stage"
)

func TestParseTransformAfterColon(t *testing.T) {
	p, err := pipeline.Parse([]string{"data.json", "::", "flatten"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Stages) != 2 {
		t.Fatalf("stages %d", len(p.Stages))
	}
	if p.Stages[0].ID != "json-src" {
		t.Fatalf("first %s", p.Stages[0].ID)
	}
	if p.Stages[1].ID != "flatten" || p.Stages[1].Kind != stage.Transform {
		t.Fatalf("second %+v", p.Stages[1])
	}
}

func TestParseExtension(t *testing.T) {
	p, err := pipeline.Parse([]string{"data.json"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if p.Stages[0].ID != "json-src" {
		t.Fatalf("got %s", p.Stages[0].ID)
	}
	opts := p.Stages[0].Options.(json.SrcOpts)
	if opts.File != "data.json" {
		t.Fatalf("file %q", opts.File)
	}
}

func TestParseJsonAliasWithPath(t *testing.T) {
	p, err := pipeline.Parse([]string{"json", "my.json"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	opts := p.Stages[0].Options.(json.SrcOpts)
	if opts.File != "my.json" {
		t.Fatalf("file %q", opts.File)
	}
}

func TestParseInlineJSON(t *testing.T) {
	p, err := pipeline.Parse([]string{`[{"x":1}]`}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if p.Stages[0].ID != "json-src" {
		t.Fatalf("id %s", p.Stages[0].ID)
	}
	opts := p.Stages[0].Options.(json.SrcOpts)
	if opts.File != `[{"x":1}]` {
		t.Fatalf("file %q", opts.File)
	}
}
