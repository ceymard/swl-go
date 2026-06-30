package cli_test

import (
	"testing"

	"github.com/ceymard/swl-go/internal/cli"
)

func TestExpandFlags(t *testing.T) {
	got := cli.ExpandFlags([]string{"-xu", "--foo=bar", "file"})
	want := []string{"-x", "-u", "--foo", "bar", "file"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}
