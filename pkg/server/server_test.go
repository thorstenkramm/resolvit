package server

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func getAvailablePort() (int, error) {
	addr, err := net.ResolveTCPAddr("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}

	l, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func TestServer(t *testing.T) {
	port, err := getAvailablePort()
	if err != nil {
		t.Fatalf("Failed to get available port: %v", err)
	}
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	logger := slog.Default()
	srv := New(addr, []string{"8.8.8.8:53"}, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel() // This ensures context cancellation even if test fails

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Run tests...
	runDNSTests(t, addr)

	// Proper shutdown sequence
	cancel() // First cancel context to stop workers
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		t.Errorf("Server shutdown failed: %v", err)
	}
}

func runDNSTests(t *testing.T, addr string) {
	t.Helper()
	// Wait for server to start
	time.Sleep(100 * time.Millisecond)

	// Test DNS queries
	tests := []struct {
		name     string
		query    string
		qtype    uint16
		wantResp bool
	}{
		{
			name:     "Valid A query",
			query:    "example.com.",
			qtype:    dns.TypeA,
			wantResp: true,
		},
		{
			name:     "Valid CNAME query",
			query:    "www.example.com.",
			qtype:    dns.TypeCNAME,
			wantResp: true,
		},
	}

	c := new(dns.Client)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := new(dns.Msg)
			m.SetQuestion(tt.query, tt.qtype)

			r, _, err := c.Exchange(m, addr)
			if err != nil {
				t.Errorf("DNS query failed: %v", err)
			}
			if (r != nil) != tt.wantResp {
				t.Errorf("Got response %v, want %v", r != nil, tt.wantResp)
			}
		})
	}
}
