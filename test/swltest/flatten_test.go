package swltest_test

import (
	"testing"

	"github.com/ceymard/swl-go/handler/flatten"
	"github.com/ceymard/swl-go/internal/coll"
)

func TestFlatten(t *testing.T) {
	row := coll.Row{"user": map[string]any{"name": "Bob", "age": 30}}
	got := flatten.Flatten(row)
	if got["user.name"] != "Bob" || got["user.age"] != 30 {
		t.Fatalf("got %v", got)
	}
}

func TestUnflatten(t *testing.T) {
	row := coll.Row{"user.name": "Bob"}
	got := flatten.Unflatten(row, false)
	if got["name"] != "Bob" {
		t.Fatalf("got %v", got)
	}
}
