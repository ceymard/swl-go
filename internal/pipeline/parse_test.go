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

func TestParseExplicitSourceAfterColon(t *testing.T) {
	p, err := pipeline.Parse([]string{"data.json", "::", "+csv", "orders.csv", "::", "out.db"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Stages) != 3 {
		t.Fatalf("stages %d", len(p.Stages))
	}
	if p.Stages[0].ID != "json-src" || p.Stages[0].Kind != stage.Source {
		t.Fatalf("first %+v", p.Stages[0])
	}
	if p.Stages[1].ID != "csv-src" || p.Stages[1].Kind != stage.Source {
		t.Fatalf("second %+v", p.Stages[1])
	}
	if p.Stages[2].ID != "sqlite-sink" || p.Stages[2].Kind != stage.Sink {
		t.Fatalf("third %+v", p.Stages[2])
	}
}

func TestParseExplicitSourceAfterColonSameAsPlusPlus(t *testing.T) {
	legacy, err := pipeline.Parse([]string{"data.json", "++", "orders.csv", "::", "out.db"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	modern, err := pipeline.Parse([]string{"data.json", "::", "+csv", "orders.csv", "::", "out.db"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(legacy.Stages) != len(modern.Stages) {
		t.Fatalf("legacy %d modern %d", len(legacy.Stages), len(modern.Stages))
	}
	for i := range legacy.Stages {
		if legacy.Stages[i].ID != modern.Stages[i].ID || legacy.Stages[i].Kind != modern.Stages[i].Kind {
			t.Fatalf("stage %d legacy %+v modern %+v", i, legacy.Stages[i], modern.Stages[i])
		}
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
