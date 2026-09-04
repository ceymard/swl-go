package progress

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/msg"
)

func TestTrackFinalRowCount(t *testing.T) {
	var buf bytes.Buffer
	log := &msg.Log{Out: &buf, Verbose: 2}

	in := func(yield func(coll.Collection, error) bool) {
		rows := func(yieldBatch func([]coll.Row, error) bool) {
			batch := []coll.Row{{0}, {1}, {2}}
			yieldBatch(batch, nil)
		}
		yield(coll.Collection{Name: "users", Rows: rows}, nil)
	}

	for c, err := range Track(log, Sink, in) {
		if err != nil {
			t.Fatal(err)
		}
		for batch, err := range c.Rows {
			if err != nil {
				t.Fatal(err)
			}
			_ = batch
		}
	}

	out := buf.String()
	if !strings.Contains(out, "users") || !strings.Contains(out, "received") || !strings.Contains(out, "3") {
		t.Fatalf("expected final sink count line, got:\n%s", out)
	}
}

func TestHandlerLogPrefix(t *testing.T) {
	var buf bytes.Buffer
	log := &msg.Log{Out: &buf, Verbose: 1}
	h := NewHandler(log, Source, "pg-src")
	h.Log(1, "connected to", "postgres://localhost/db")
	if !strings.Contains(buf.String(), "pg") {
		t.Fatalf("expected handler name in log: %q", buf.String())
	}
}
