package flatten_test

import (
	"testing"

	"github.com/ceymard/swl-go/handler/flatten"
)

func TestFlattenNestedMap(t *testing.T) {
	got := flatten.Flatten(map[string]any{
		"user": map[string]any{"name": "Ann", "age": 30},
	})
	if got["user.name"] != "Ann" || got["user.age"] != 30 {
		t.Fatalf("got %+v", got)
	}
}

func TestFlattenArray(t *testing.T) {
	got := flatten.Flatten(map[string]any{
		"tags": []any{"a", "b"},
	})
	if got["tags[0]"] != "a" || got["tags[1]"] != "b" {
		t.Fatalf("got %+v", got)
	}
}

func TestUnflattenRoundTrip(t *testing.T) {
	flat := flatten.Flatten(map[string]any{
		"x": map[string]any{"y": 1},
	})
	back := flatten.Unflatten(flat, false)
	if back["y"] != 1 {
		t.Fatalf("got %+v", back)
	}
}

func TestUnflattenDropEmpty(t *testing.T) {
	got := flatten.Unflatten(map[string]any{
		"a.b": "",
		"a.c": 1,
	}, true)
	if got["c"] != 1 {
		t.Fatalf("got %+v", got)
	}
	if v, ok := got["b"]; ok && v != "" && v != nil {
		t.Fatalf("expected empty b dropped or nil, got %v", v)
	}
}
