package handlers_test

import (
	"context"
	"errors"
	"testing"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/handlers"
	"github.com/ceymard/swl-go/internal/stream"
)

type mockHooks struct {
	writes int
}

func (m *mockHooks) Init(ctx context.Context) error { return nil }
func (m *mockHooks) Rollback(ctx context.Context)   {}
func (m *mockHooks) Finish(ctx context.Context) error {
	return errors.New("finish should not succeed after write error")
}

func (m *mockHooks) Open(ctx context.Context, col coll.Collection, firstRow coll.Row) (handlers.RowWriter, error) {
	return m, nil
}

func (m *mockHooks) Write(row coll.Row) error {
	m.writes++
	return errors.New("boom")
}

func (m *mockHooks) Close() error { return nil }

func TestConsumeHooksRollbackOnWriteError(t *testing.T) {
	in := stream.Of(coll.Collection{
		Name: "t",
		Rows: coll.SliceRowBatches([]coll.Row{{1}, {2}}),
	})
	h := &mockHooks{}
	err := handlers.ConsumeHooks(handlers.Config{Ctx: context.Background()}, h, in)
	if err == nil {
		t.Fatal("expected write error")
	}
	if h.writes != 1 {
		t.Fatalf("writes %d", h.writes)
	}
}
