package handler

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"resolvit/internal/testutil"
	"resolvit/pkg/dnscache"
	"resolvit/pkg/filtering"
	"resolvit/pkg/forward"
	"resolvit/pkg/records"

	"github.com/miekg/dns"
)

func TestHandleDNSRequestFiltering(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	tmpDir := t.TempDir()
	blocklistPath := filepath.Join(tmpDir, "blocklist.txt")
	allowlistPath := filepath.Join(tmpDir, "allowlist.txt")

	blocklistData := []byte("blocked.example.com\nblocked2.example.com\noverride.example.com\n")
	allowlistData := []byte("override.example.com\n")
	if err := os.WriteFile(blocklistPath, blocklistData, 0o600); err != nil {
		t.Fatalf("write blocklist: %v", err)
	}
	if err := os.WriteFile(allowlistPath, allowlistData, 0o600); err != nil {
		t.Fatalf("write allowlist: %v", err)
	}

	filter := filtering.NewFilter(filtering.FilterOptions{
		Enabled:       true,
		AllowlistPath: allowlistPath,
		Sources: []filtering.Source{
			{ID: "test", Location: blocklistPath, Enabled: true},
		},
		Log:        logger,
		ErrorLimit: 5,
	})
	filter.LoadOnce(context.Background())

	responses := map[string]testutil.Response{
		testutil.Key("allowed.example.com.", dns.TypeA): {
			Answers: []dns.RR{testutil.ARecord("allowed.example.com.", "192.0.2.10")},
		},
		testutil.Key("override.example.com.", dns.TypeA): {
			Answers: []dns.RR{testutil.ARecord("override.example.com.", "192.0.2.11")},
		},
	}
	stub := testutil.StartDNSStub(t, testutil.FixedHandler(responses))

	cache := dnscache.New(logger)
	forwarder := forward.New([]string{stub.Addr}, logger)
	h := New(cache, forwarder, "127.0.0.1:5300", logger, filter)

	records.Add("blocked.example.com.", records.A, "192.0.2.20")
	cleanupPath := filepath.Join(tmpDir, "empty-records.txt")
	if err := os.WriteFile(cleanupPath, []byte(""), 0o600); err != nil {
		t.Fatalf("write cleanup records: %v", err)
	}
	t.Cleanup(func() {
		_ = records.LoadFromFile(cleanupPath, logger)
	})

	blockedMsg := sendQuery(t, h, "blocked2.example.com.")
	if blockedMsg.Rcode != dns.RcodeNameError {
		t.Fatalf("expected NXDOMAIN for blocked2.example.com, got %d", blockedMsg.Rcode)
	}

	allowedMsg := sendQuery(t, h, "allowed.example.com.")
	if len(allowedMsg.Answer) == 0 {
		t.Fatal("expected answer for allowed.example.com")
	}

	overrideMsg := sendQuery(t, h, "override.example.com.")
	if len(overrideMsg.Answer) == 0 {
		t.Fatal("expected answer for override.example.com")
	}

	localMsg := sendQuery(t, h, "blocked.example.com.")
	if localMsg.Rcode != dns.RcodeSuccess {
		t.Fatalf("expected success for local record, got %d", localMsg.Rcode)
	}
	if len(localMsg.Answer) == 0 {
		t.Fatal("expected local answer for blocked.example.com")
	}
	if aRecord, ok := localMsg.Answer[0].(*dns.A); !ok || aRecord.A.String() != "192.0.2.20" {
		t.Fatal("expected local A record for blocked.example.com")
	}
}

func sendQuery(t *testing.T, h *Handler, name string) *dns.Msg {
	t.Helper()
	req := new(dns.Msg)
	req.SetQuestion(name, dns.TypeA)
	w := &testResponseWriter{network: "udp"}
	h.HandleDNSRequest(w, req)
	if w.msg == nil {
		t.Fatal("expected response message")
	}
	return w.msg
}
