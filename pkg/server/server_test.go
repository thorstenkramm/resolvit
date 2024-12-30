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
		addr      string
		upstreams []string
	}{
		{
			name:      "valid server configuration",
			addr:      "127.0.0.1:5353",
			upstreams: []string{"8.8.8.8:53", "8.8.4.4:53"},
		},
		{
			name:      "server with single upstream",
			addr:      "127.0.0.1:5354",
			upstreams: []string{"1.1.1.1:53"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			srv := New(tt.addr, tt.upstreams, logger)

			if srv == nil {
				t.Fatal("expected non-nil server")
			}

			if srv.server.Addr != tt.addr {
				t.Errorf("expected address %s, got %s", tt.addr, srv.server.Addr)
			}

			if srv.server.Net != "udp" {
				t.Errorf("expected UDP network, got %s", srv.server.Net)
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
	srv := New("127.0.0.1:5355", []string{"8.8.8.8:53"}, logger)

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
