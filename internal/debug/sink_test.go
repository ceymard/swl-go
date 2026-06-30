package debug

import (
	"bytes"
	"strings"
	"testing"

	"github.com/ceymard/swl-go/internal/coll"
	"github.com/ceymard/swl-go/internal/stream"
)

func TestPrintRowPlain(t *testing.T) {
	var buf bytes.Buffer
	s := stream.Of(coll.Collection{
		Name: "users",
		Rows: coll.SliceRows([]coll.Row{{
			"name": "alice",
			"id":   float64(1),
			"ok":   true,
		}}),
	})
	if err := sinkTo(&buf, 0, s); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"users:1 ", "name", "alice", "id", "1", "ok", "true"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in %q", want, out)
		}
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("unexpected ANSI in plain output: %q", out)
	}
}
