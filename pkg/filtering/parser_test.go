package filtering

import (
	"bytes"
	"io"
	"log/slog"
	"strings"
	"testing"
)

func TestParseListHostsAndDomains(t *testing.T) {
	input := strings.Join([]string{
		"# Comment line",
		"127.0.0.1 bad.example.com",
		"0.0.0.0 also.bad.example.com # trailing comment",
		"bad.example.net",
		"*.wild.example.org",
		"; another comment",
		"",
	}, "\n")

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	set, err := parseList(strings.NewReader(input), parseOptions{
		ListID:     "test",
		Logger:     logger,
		ErrorLimit: 5,
	})
	if err != nil {
		t.Fatalf("parseList returned error: %v", err)
	}

	if !set.Matches("bad.example.com.", false) {
		t.Error("expected bad.example.com to be blocked")
	}
	if !set.Matches("also.bad.example.com.", false) {
		t.Error("expected also.bad.example.com to be blocked")
	}
	if !set.Matches("bad.example.net.", false) {
		t.Error("expected bad.example.net to be blocked")
	}
	if !set.Matches("host.wild.example.org", false) {
		t.Error("expected wildcard to block host.wild.example.org")
	}
}

func TestParseListErrorLimit(t *testing.T) {
	input := strings.Join([]string{
		"good.example.com",
		"http://bad.example.com",
		"1.2.3.4",
		"bad..example.com",
		"foo/bar",
	}, "\n")

	var logBuf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	_, err := parseList(strings.NewReader(input), parseOptions{
		ListID:     "test",
		Logger:     logger,
		ErrorLimit: 2,
	})
	if err != nil {
		t.Fatalf("parseList returned error: %v", err)
	}

	logText := logBuf.String()
	if got := strings.Count(logText, "invalid blocklist entry"); got != 2 {
		t.Fatalf("expected 2 invalid entry logs, got %d", got)
	}
	if !strings.Contains(logText, "blocklist parsing errors suppressed") {
		t.Error("expected summary log for suppressed errors")
	}
}
