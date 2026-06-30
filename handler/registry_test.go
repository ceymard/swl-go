package handler_test

import (
	"strings"
	"testing"

	"github.com/ceymard/swl-go/handler"
)

func TestListAvailableSections(t *testing.T) {
	out := handler.ListAvailable()
	for _, heading := range []string{"handlers:", "extensions:", "protocols:"} {
		if !strings.Contains(out, heading) {
			t.Fatalf("missing %q in:\n%s", heading, out)
		}
	}
	if !strings.Contains(out, "  ⇄ json\n") {
		t.Fatalf("expected json handler, got:\n%s", out)
	}
	if !strings.Contains(out, "  → flatten\n") {
		t.Fatalf("expected flatten transform, got:\n%s", out)
	}
	if !strings.Contains(out, "  ⇄ .json\n") {
		t.Fatalf("expected .json extension, got:\n%s", out)
	}
}

func TestListAvailableSorted(t *testing.T) {
	out := handler.ListAvailable()
	iCoerce := strings.Index(out, "  → coerce\n")
	iCsv := strings.Index(out, "  ⇄ csv\n")
	iFlatten := strings.Index(out, "  → flatten\n")
	if iCoerce < 0 || iCsv < 0 || iFlatten < 0 {
		t.Fatalf("missing entries:\n%s", out)
	}
	if !(iCoerce < iCsv && iCsv < iFlatten) {
		t.Fatalf("handlers not sorted:\n%s", out)
	}
}
