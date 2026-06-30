package handler_test

import (
	"strings"
	"testing"

	"github.com/ceymard/swl-go/handler"
)

func TestHelpForArgvSQLiteSinkDefault(t *testing.T) {
	text, ok, err := handler.HelpForArgv([]string{"sqlite", "--help"}, "swl")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if !strings.Contains(text, "swl sqlite") {
		t.Fatalf("missing usage line: %q", text)
	}
	if !strings.Contains(text, "-t") || !strings.Contains(text, "--truncate") {
		t.Fatalf("expected sqlite-sink flags, got:\n%s", text)
	}
}

func TestHelpForArgvPgSourceExplicit(t *testing.T) {
	text, ok, err := handler.HelpForArgv([]string{"+pg", "--help"}, "swl")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if !strings.Contains(text, "swl +pg") {
		t.Fatalf("missing usage: %q", text)
	}
	if !strings.Contains(text, "-s") || !strings.Contains(text, "--schema") {
		t.Fatalf("expected pg-src flags, got:\n%s", text)
	}
}

func TestHelpForArgvPgSinkDefault(t *testing.T) {
	text, ok, err := handler.HelpForArgv([]string{"pg", "--help"}, "swl")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if !strings.Contains(text, "--auto-create") {
		t.Fatalf("expected pg-sink flags, got:\n%s", text)
	}
}

func TestHelpForArgvPgSourceAfterColon(t *testing.T) {
	text, ok, err := handler.HelpForArgv([]string{"data.json", "::", "+pg", "--help"}, "swl")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if !strings.Contains(text, "swl +pg") {
		t.Fatalf("missing usage: %q", text)
	}
	if !strings.Contains(text, "--schema") {
		t.Fatalf("expected pg-src flags, got:\n%s", text)
	}
}

func TestHelpForArgvPipelineSinkSegment(t *testing.T) {
	text, ok, err := handler.HelpForArgv([]string{"data.json", "::", "sqlite", "--help"}, "swl")
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
	if !strings.Contains(text, "-t") {
		t.Fatalf("expected sqlite-sink help, got:\n%s", text)
	}
}

func TestHelpForArgvNoHandlerHelp(t *testing.T) {
	_, ok, err := handler.HelpForArgv([]string{"data.json", "::", "out.db"}, "swl")
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}
