package swltest_test

import (
	"context"
	"testing"

	"github.com/ceymard/swl-go/handler"
	"github.com/ceymard/swl-go/handler/flatten"
	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/handlers"
	"github.com/ceymard/swl-go/internal/pipeline"
	"github.com/ceymard/swl-go/internal/runner"
	"github.com/ceymard/swl-go/internal/stage"
	"github.com/ceymard/swl-go/test/swltest"
)

func TestMain(m *testing.M) {
	swltest.RegisterHandlers()
	m.Run()
}

func TestFlattenPipeline(t *testing.T) {
	src := swltest.MemSource{Collections: []coll.Collection{{
		Name: "users",
		Rows: coll.SliceRowBatches([]coll.Row{{"user": map[string]any{"name": "Ann"}}}),
	}}}
	handler.Register(swltest.MemSrcID, src, handler.Meta{})

	var got []coll.Row
	handler.Register(swltest.CollectSinkID, swltest.RowCollectSink{Rows: &got}, handler.Meta{})
	handler.RegisterParser(swltest.CollectSinkID, func(_ string, _ []string) (any, error) {
		return struct{}{}, nil
	})

	p := pipeline.Pipeline{
		Stages: []pipeline.Stage{
			{Kind: stage.Source, ID: swltest.MemSrcID, Options: swltest.MemOptions{}},
			{Kind: stage.Transform, ID: "flatten", Options: flatten.Options{}},
			{Kind: stage.Sink, ID: swltest.CollectSinkID},
		},
	}
	cfg := handlers.Config{Ctx: context.Background()}
	if err := runner.Run(cfg, handler.Reg, p); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0]["user.name"] != "Ann" {
		t.Fatalf("got %+v", got)
	}
}
