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
	src := swltest.MemSource{Collections: []coll.Collection{
		swltest.Coll("users", []string{"user"}, []any{map[string]any{"name": "Ann"}}),
	}}
	handler.Register(swltest.MemSrcID, src, handler.Meta{})

	var snaps []swltest.Snapshot
	handler.Register(swltest.CollectSinkID, swltest.RowCollectSink{Snaps: &snaps}, handler.Meta{})
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
	if len(snaps) != 1 || len(snaps[0].Rows) != 1 || snaps[0].Cell(0, "user.name") != "Ann" {
		t.Fatalf("got %+v", snaps)
	}
}
