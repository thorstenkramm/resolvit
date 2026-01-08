package filtering

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/miekg/dns"
)

func writeTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}

func TestFilterAllowlistOverride(t *testing.T) {
	tmpDir := t.TempDir()
	blocklistPath := writeTempFile(t, tmpDir, "blocklist.txt", strings.Join([]string{
		"blocked.example.com",
		"allow.example.com",
		"*.wild.example.net",
	}, "\n"))
	allowlistPath := writeTempFile(t, tmpDir, "allowlist.txt", strings.Join([]string{
		"allow.example.com",
		"*.safe.example.com",
	}, "\n"))

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	filter := NewFilter(FilterOptions{
		Enabled:       true,
		AllowlistPath: allowlistPath,
		Sources: []Source{
			{ID: "test", Location: blocklistPath, Enabled: true},
		},
		Log:        logger,
		ErrorLimit: 5,
	})
	filter.LoadOnce(context.Background())

	if !filter.ShouldBlock("blocked.example.com") {
		t.Error("expected blocked.example.com to be blocked")
	}
	if filter.ShouldBlock("allow.example.com") {
		t.Error("expected allow.example.com to be allowed")
	}
	if filter.ShouldBlock("host.safe.example.com") {
		t.Error("expected host.safe.example.com to be allowed")
	}
	if !filter.ShouldBlock("host.wild.example.net") {
		t.Error("expected host.wild.example.net to be blocked")
	}
}

func TestFilterBlockSubdomains(t *testing.T) {
	tmpDir := t.TempDir()
	blocklistPath := writeTempFile(t, tmpDir, "blocklist.txt", "example.com\n")

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	filterExact := NewFilter(FilterOptions{
		Enabled:         true,
		BlockSubdomains: false,
		Sources: []Source{
			{ID: "test", Location: blocklistPath, Enabled: true},
		},
		Log:        logger,
		ErrorLimit: 5,
	})
	filterExact.LoadOnce(context.Background())

	if !filterExact.ShouldBlock("example.com") {
		t.Error("expected example.com to be blocked")
	}
	if filterExact.ShouldBlock("sub.example.com") {
		t.Error("did not expect sub.example.com to be blocked when subdomains are off")
	}

	filterSub := NewFilter(FilterOptions{
		Enabled:         true,
		BlockSubdomains: true,
		Sources: []Source{
			{ID: "test", Location: blocklistPath, Enabled: true},
		},
		Log:        logger,
		ErrorLimit: 5,
	})
	filterSub.LoadOnce(context.Background())

	if !filterSub.ShouldBlock("sub.example.com") {
		t.Error("expected sub.example.com to be blocked when subdomains are on")
	}
}

func TestFilterBlockedLogWrites(t *testing.T) {
	tmpDir := t.TempDir()
	blockedLog := filepath.Join(tmpDir, "blocked.log")

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	filter := NewFilter(FilterOptions{
		Enabled:        true,
		BlockedLogPath: blockedLog,
		Log:            logger,
		ErrorLimit:     5,
	})

	filter.LogBlocked("127.0.0.1:1053", "blocked.example.com.", dns.TypeA)

	data, err := os.ReadFile(blockedLog) // #nosec G304 -- test temp file path.
	if err != nil {
		t.Fatalf("read blocked log: %v", err)
	}
	logText := string(data)
	if !strings.Contains(logText, "blocked.example.com") {
		t.Error("expected blocked domain to appear in blocked log")
	}
	if !strings.Contains(logText, "client=127.0.0.1:1053") {
		t.Error("expected client address to appear in blocked log")
	}
}
