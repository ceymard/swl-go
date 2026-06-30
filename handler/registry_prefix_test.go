package handler_test

import (
	"testing"

	"github.com/ceymard/swl-go/handler"
	"github.com/ceymard/swl-go/internal/stage"
)

func TestSplitSourcePrefixExplicit(t *testing.T) {
	name, explicit := handler.SplitSourcePrefix("+pg")
	if name != "pg" || !explicit {
		t.Fatalf("name=%q explicit=%v", name, explicit)
	}
}

func TestSplitSourcePrefixPlain(t *testing.T) {
	name, explicit := handler.SplitSourcePrefix("sqlite")
	if name != "sqlite" || explicit {
		t.Fatalf("name=%q explicit=%v", name, explicit)
	}
}

func TestSplitSourcePrefixPlusOnly(t *testing.T) {
	name, explicit := handler.SplitSourcePrefix("+")
	if name != "+" || explicit {
		t.Fatalf("name=%q explicit=%v", name, explicit)
	}
}

func TestResolveAliasExplicitSourceAfterColonSemantics(t *testing.T) {
	// Registry resolves alias; pipeline applies + prefix before calling.
	id, kind, ok := handler.ResolveAlias("pg", false)
	if !ok || id != "pg-src" || kind != stage.Source {
		t.Fatalf("pg-src: id=%s kind=%v ok=%v", id, kind, ok)
	}
	id, kind, ok = handler.ResolveAlias("pg", true)
	if !ok || id != "pg-sink" || kind != stage.Sink {
		t.Fatalf("pg-sink: id=%s kind=%v ok=%v", id, kind, ok)
	}
}
