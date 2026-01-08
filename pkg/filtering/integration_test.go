//go:build integration
// +build integration

package filtering_test

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"resolvit/internal/testutil"
	"resolvit/pkg/dnscache"
	"resolvit/pkg/filtering"
	"resolvit/pkg/forward"
	"resolvit/pkg/handler"

	"github.com/miekg/dns"
)

type captureWriter struct {
	msg *dns.Msg
}

func (w *captureWriter) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 53}
}

func (w *captureWriter) RemoteAddr() net.Addr {
	return &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1053}
}

func (w *captureWriter) WriteMsg(msg *dns.Msg) error {
	w.msg = msg
	return nil
}

func (w *captureWriter) Write([]byte) (int, error) {
	return 0, nil
}

func (w *captureWriter) Close() error {
	return nil
}

func (w *captureWriter) TsigStatus() error {
	return nil
}

func (w *captureWriter) TsigTimersOnly(bool) {
}

func (w *captureWriter) Hijack() {
}

func TestIntegrationRealListBlocksDomain(t *testing.T) {
	def, ok := filtering.Catalog["blocklistproject_malware"]
	if !ok {
		t.Fatal("blocklistproject_malware not found in catalog")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	data, err := downloadList(ctx, def.URL)
	if err != nil {
		t.Fatalf("download list: %v", err)
	}

	blockedDomain, err := firstDomainFromList(data)
	if err != nil {
		t.Fatalf("find domain from list: %v", err)
	}

	listPath := filepath.Join(t.TempDir(), "list.txt")
	if err := os.WriteFile(listPath, data, 0o600); err != nil {
		t.Fatalf("write list: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	filter := filtering.NewFilter(filtering.FilterOptions{
		Enabled: true,
		Sources: []filtering.Source{{
			ID:       "blocklistproject_malware",
			Location: listPath,
			Enabled:  true,
		}},
		Log: logger,
	})
	filter.LoadOnce(context.Background())

	responses := map[string]testutil.Response{
		testutil.Key("example.com.", dns.TypeA): {
			Answers: []dns.RR{testutil.ARecord("example.com.", "93.184.216.34")},
		},
	}
	stub := testutil.StartDNSStub(t, testutil.FixedHandler(responses))

	cache := dnscache.New(logger)
	forwarder := forward.New([]string{stub.Addr}, logger)
	h := handler.New(cache, forwarder, "127.0.0.1:5300", logger, filter)

	blockedMsg := sendQuery(t, h, dns.Fqdn(blockedDomain))
	if blockedMsg.Rcode != dns.RcodeNameError {
		t.Fatalf("expected NXDOMAIN for %s, got %d", blockedDomain, blockedMsg.Rcode)
	}

	allowedMsg := sendQuery(t, h, "example.com.")
	if len(allowedMsg.Answer) == 0 {
		t.Fatal("expected answer for example.com")
	}
}

func sendQuery(t *testing.T, h *handler.Handler, name string) *dns.Msg {
	t.Helper()
	req := new(dns.Msg)
	req.SetQuestion(name, dns.TypeA)
	w := &captureWriter{}
	h.HandleDNSRequest(w, req)
	if w.msg == nil {
		t.Fatal("expected response message")
	}
	return w.msg
}

func downloadList(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

func firstDomainFromList(data []byte) (string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := strings.TrimSpace(strings.TrimPrefix(scanner.Text(), "\ufeff"))
		if line == "" || isCommentLine(line) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}

		candidates := fields
		if net.ParseIP(fields[0]) != nil {
			candidates = fields[1:]
		}
		for _, token := range candidates {
			if isCommentToken(token) {
				break
			}
			normalized, wildcard := normalizeToken(token)
			if normalized == "" {
				continue
			}
			if wildcard {
				return "sub." + normalized, nil
			}
			return normalized, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", fmt.Errorf("no domain found in list")
}

func normalizeToken(token string) (string, bool) {
	trimmed := strings.TrimSpace(token)
	if trimmed == "" {
		return "", false
	}
	if strings.Contains(trimmed, "://") || strings.Contains(trimmed, "/") || strings.Contains(trimmed, ":") {
		return "", false
	}
	if net.ParseIP(trimmed) != nil {
		return "", false
	}

	wildcard := false
	if strings.HasPrefix(trimmed, "*.") {
		wildcard = true
		trimmed = strings.TrimPrefix(trimmed, "*.")
	}
	trimmed = strings.TrimSuffix(strings.ToLower(trimmed), ".")
	if trimmed == "" {
		return "", false
	}
	if _, ok := dns.IsDomainName(trimmed); !ok {
		return "", false
	}
	return trimmed, wildcard
}

func isCommentLine(line string) bool {
	return strings.HasPrefix(line, "#") || strings.HasPrefix(line, "//") || strings.HasPrefix(line, ";")
}

func isCommentToken(token string) bool {
	return strings.HasPrefix(token, "#") || strings.HasPrefix(token, "//") || strings.HasPrefix(token, ";")
}
