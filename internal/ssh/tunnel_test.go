package ssh_test

import (
	"strings"
	"testing"

	"github.com/ceymard/swl-go/internal/ssh"
)

func TestMaybeOpenTunnelNoPattern(t *testing.T) {
	uri := "postgres://user:pass@localhost:5432/mydb"
	got, err := ssh.MaybeOpenTunnel(uri, 5432)
	if err != nil {
		t.Fatal(err)
	}
	if got.URI != uri {
		t.Fatalf("uri %q", got.URI)
	}
	if got.Tunnel != nil {
		t.Fatal("expected no tunnel")
	}
}

func TestMaybeOpenTunnelReplaceURI(t *testing.T) {
	// Cannot open real tunnel in unit test; verify regex replacement via unexported pattern
	// by checking error is connect-related, not parse-related.
	uri := "postgres://u:p@dbhost:5432@@jump.example/mydb"
	got, err := ssh.MaybeOpenTunnel(uri, 5432)
	if err == nil {
		got.Close()
		if !strings.Contains(got.URI, "127.0.0.1:") {
			t.Fatalf("expected localhost forward in %q", got.URI)
		}
		return
	}
	// Without ssh server, expect dial error — uri must still be rewritten only after success.
	if !strings.Contains(err.Error(), "ssh connect") {
		t.Fatalf("unexpected error: %v", err)
	}
}
