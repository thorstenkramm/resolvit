// Package server exposes the DNS server wiring for resolvit.
package server

import (
	"log/slog"
	"resolvit/pkg/dnscache"
	"resolvit/pkg/filtering"
	"resolvit/pkg/forward"
	"resolvit/pkg/handler"
	"resolvit/pkg/version"

	"github.com/miekg/dns"
)

// Server hosts UDP and TCP DNS servers sharing the same handler stack.
// Multiple listen addresses are supported; each address gets its own
// UDP/TCP server pair.
type Server struct {
	servers    []*dns.Server
	tcpServers []*dns.Server
	cache      *dnscache.DNSCache
	forwarder  *forward.Forwarder
	log        *slog.Logger
}

// New constructs a Server that listens on addrs and forwards to upstreams.
func New(addrs []string, upstreams []string, log *slog.Logger, filter *filtering.Filter) *Server {
	cache := dnscache.New(log)
	forwarder := forward.New(upstreams, log)

	s := &Server{
		cache:     cache,
		forwarder: forwarder,
		log:       log,
	}

	for _, addr := range addrs {
		s.servers = append(s.servers, &dns.Server{Addr: addr, Net: "udp", UDPSize: 65535})
		s.tcpServers = append(s.tcpServers, &dns.Server{Addr: addr, Net: "tcp"})
	}

	dnsHandler := handler.New(cache, forwarder, log, filter)
	dns.HandleFunc(".", dnsHandler.HandleDNSRequest)

	return s
}

// Start launches all TCP and UDP listeners and blocks until any server fails.
func (s *Server) Start() error {
	for _, srv := range s.servers {
		s.log.Info("starting DNS server", "version", version.ResolvitVersion, "address", srv.Addr)
	}

	errCh := make(chan error, 1)

	for _, srv := range s.tcpServers {
		go func() {
			if err := srv.ListenAndServe(); err != nil {
				s.log.Error("TCP server error", "address", srv.Addr, "error", err)
				errCh <- err
			}
		}()
	}

	for _, srv := range s.servers {
		go func() {
			if err := srv.ListenAndServe(); err != nil {
				errCh <- err
			}
		}()
	}

	return <-errCh
}

// ClearCache removes all cached DNS entries.
func (s *Server) ClearCache() {
	s.cache.Clear()
}
