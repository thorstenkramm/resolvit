package server

import (
	"io"
	"log/slog"
	"testing"
	"time"
)

func TestNewServer(t *testing.T) {
	tests := []struct {
		name      string
		addrs     []string
		upstreams []string
	}{
		{
			name:      "valid server configuration",
			addrs:     []string{"127.0.0.1:5353"},
			upstreams: []string{"8.8.8.8:53", "8.8.4.4:53"},
		},
		{
			name:      "server with single upstream",
			addrs:     []string{"127.0.0.1:5354"},
			upstreams: []string{"1.1.1.1:53"},
		},
		{
			name:      "server with multiple listen addresses",
			addrs:     []string{"127.0.0.1:5355", "127.0.0.1:5356"},
			upstreams: []string{"8.8.8.8:53"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			srv := New(tt.addrs, tt.upstreams, logger, nil)

			if srv == nil {
				t.Fatal("expected non-nil server")
			}

			if len(srv.servers) != len(tt.addrs) {
				t.Errorf("expected %d UDP servers, got %d", len(tt.addrs), len(srv.servers))
			}

			if len(srv.tcpServers) != len(tt.addrs) {
				t.Errorf("expected %d TCP servers, got %d", len(tt.addrs), len(srv.tcpServers))
			}

			for i, addr := range tt.addrs {
				if srv.servers[i].Addr != addr {
					t.Errorf("expected UDP address %s, got %s", addr, srv.servers[i].Addr)
				}
				if srv.servers[i].Net != "udp" {
					t.Errorf("expected UDP network, got %s", srv.servers[i].Net)
				}
				if srv.tcpServers[i].Addr != addr {
					t.Errorf("expected TCP address %s, got %s", addr, srv.tcpServers[i].Addr)
				}
			}

			if srv.cache == nil {
				t.Error("expected non-nil cache")
			}

			if srv.forwarder == nil {
				t.Error("expected non-nil forwarder")
			}
		})
	}
}

func TestServerStart(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	srv := New([]string{"127.0.0.1:5355"}, []string{"8.8.8.8:53"}, logger, nil)

	errChan := make(chan error)
	go func() {
		errChan <- srv.Start()
	}()

	// Give the server time to start
	time.Sleep(100 * time.Millisecond)

	select {
	case err := <-errChan:
		t.Fatalf("server failed to start: %v", err)
	default:
		// Server started successfully
	}
}
