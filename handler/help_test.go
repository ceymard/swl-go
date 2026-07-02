package handler_test

import (
	"strings"
	"testing"

	"github.com/ceymard/swl-go/handler"
)

func TestHelpForArgvPgShowsBothSrcAndSink(t *testing.T) {
	text, ok, err := handler.HelpForArgv([]string{"pg", "--help"}, "swl")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if !strings.Contains(text, "SOURCE\n") || !strings.Contains(text, "SINK\n") {
		t.Fatalf("expected SOURCE and SINK sections, got:\n%s", text)
	}
	if !strings.Contains(text, "swl pg …  (chain sources with ++)") {
		t.Fatalf("missing source usage with ++, got:\n%s", text)
	}
	if !strings.Contains(text, "swl … :: pg …") {
		t.Fatalf("missing sink usage with ::, got:\n%s", text)
	}
	if !strings.Contains(text, "--schema") || !strings.Contains(text, "-q") || !strings.Contains(text, "--query") {
		t.Fatalf("expected pg-src flags, got:\n%s", text)
	}
	if !strings.Contains(text, "--auto-create") || !strings.Contains(text, "where collections: <options>") {
		t.Fatalf("expected pg-sink flags, got:\n%s", text)
	}
	// BASE SWL OPTIONS should appear once (in SOURCE section only).
	if strings.Count(text, "BASE SWL OPTIONS") != 1 {
		t.Fatalf("expected BASE SWL OPTIONS once, got %d in:\n%s", strings.Count(text, "BASE SWL OPTIONS"), text)
	}
}

func TestHelpForArgvPgExplicitPlusSameAsBare(t *testing.T) {
	bare, ok, err := handler.HelpForArgv([]string{"pg", "--help"}, "swl")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	plus, ok, err := handler.HelpForArgv([]string{"+pg", "--help"}, "swl")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if bare != plus {
		t.Fatalf("+pg and pg help differ:\n%s\n---\n%s", bare, plus)
	}
}

func TestHelpForArgvPipelineSinkSegment(t *testing.T) {
	text, ok, err := handler.HelpForArgv([]string{"data.json", "::", "sqlite", "--help"}, "swl")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if strings.Contains(text, "SOURCE\n") {
		t.Fatalf("expected sink-only help after ::, got:\n%s", text)
	}
	if !strings.Contains(text, "swl … :: sqlite …") || !strings.Contains(text, "-t") {
		t.Fatalf("expected sqlite-sink help, got:\n%s", text)
	}
}

func TestHelpForArgvPgSourceAfterColonExplicit(t *testing.T) {
	text, ok, err := handler.HelpForArgv([]string{"data.json", "::", "+pg", "--help"}, "swl")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if strings.Contains(text, "SOURCE\n") {
		t.Fatalf("expected single source help for :: +pg, got:\n%s", text)
	}
	if !strings.Contains(text, "--schema") {
		t.Fatalf("expected pg-src flags, got:\n%s", text)
	}
}

func TestHelpForArgvNoHandlerHelp(t *testing.T) {
	_, ok, err := handler.HelpForArgv([]string{"data.json", "::", "out.db"}, "swl")
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestHelpForArgvShortFlag(t *testing.T) {
	text, ok, err := handler.HelpForArgv([]string{"yaml", "-h"}, "swl")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if !strings.Contains(text, "SOURCE\n") || !strings.Contains(text, "SINK\n") {
		t.Fatalf("expected dual yaml help, got:\n%s", text)
	}
}

func TestHelpForArgvTransform(t *testing.T) {
	text, ok, err := handler.HelpForArgv([]string{"coerce", "--help"}, "swl")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if !strings.Contains(text, "only-columns") {
		t.Fatalf("expected coerce help, got:\n%s", text)
	}
}

func TestHelpForArgvUnknownHandler(t *testing.T) {
	_, ok, err := handler.HelpForArgv([]string{"nope", "--help"}, "swl")
	if !ok || err == nil {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestHelpForArgvExtensionDual(t *testing.T) {
	text, ok, err := handler.HelpForArgv([]string{"out.db", "--help"}, "swl")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if !strings.Contains(text, "SOURCE\n") || !strings.Contains(text, "SINK\n") {
		t.Fatalf("expected dual help for .db extension, got:\n%s", text)
	}
	if !strings.Contains(text, "*.db") {
		t.Fatalf("expected *.db label for extension help, got:\n%s", text)
	}
}
