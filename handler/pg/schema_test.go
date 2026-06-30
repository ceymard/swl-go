package pg

import (
	"reflect"
	"testing"
)

func TestOrderTablesByFKDeps(t *testing.T) {
	deps := map[string][]string{
		"app.accounts": {},
		"app.users":    {"app.accounts"},
		"app.posts":    {"app.users"},
	}
	got := orderTablesByFKDeps(deps)
	want := []string{"app.accounts", "app.users", "app.posts"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestOrderTablesByFKDepsDiamond(t *testing.T) {
	// a -> b, a -> c, d -> b, d -> c  => a before b/c, b/c before d
	deps := map[string][]string{
		"app.a": {},
		"app.b": {"app.a"},
		"app.c": {"app.a"},
		"app.d": {"app.b", "app.c"},
	}
	got := orderTablesByFKDeps(deps)
	index := func(name string) int {
		for i, n := range got {
			if n == name {
				return i
			}
		}
		return -1
	}
	if index("app.a") >= index("app.b") || index("app.a") >= index("app.c") {
		t.Fatalf("a must precede b and c: %v", got)
	}
	if index("app.b") >= index("app.d") || index("app.c") >= index("app.d") {
		t.Fatalf("b and c must precede d: %v", got)
	}
}

func TestOrderTablesByFKDepsSelfReferenceIgnored(t *testing.T) {
	deps := map[string][]string{
		"app.t": {"app.t"},
	}
	got := orderTablesByFKDeps(deps)
	if len(got) != 1 || got[0] != "app.t" {
		t.Fatalf("got %v", got)
	}
}
