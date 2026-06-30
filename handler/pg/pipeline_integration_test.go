package pg_test

import (
	"testing"

	"github.com/ceymard/swl-go/handler/pg"
	"github.com/ceymard/swl-go/internal/pipeline"
)

func TestIntegrationPipelinePostgresURI(t *testing.T) {
	uri := startPostgres(t)
	p, err := pipeline.Parse([]string{uri, "-s", "app"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Stages) != 1 || p.Stages[0].ID != "pg-src" {
		t.Fatalf("stages %+v", p.Stages)
	}
	opts := p.Stages[0].Options.(pg.SrcOpts)
	if opts.Schema != "app" || opts.URI != uri {
		t.Fatalf("opts %+v", opts)
	}

	p2, err := pipeline.Parse([]string{uri, "-s", "app", "::", uri, "--auto-create"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(p2.Stages) != 2 || p2.Stages[0].ID != "pg-src" || p2.Stages[1].ID != "pg-sink" {
		t.Fatalf("stages %+v", p2.Stages)
	}
	sinkOpts := p2.Stages[1].Options.(pg.SinkOpts)
	if !sinkOpts.AutoCreate || sinkOpts.URI != uri {
		t.Fatalf("sink opts %+v", sinkOpts)
	}
}
