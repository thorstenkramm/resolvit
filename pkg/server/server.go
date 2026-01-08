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

// Server hosts both UDP and TCP DNS servers sharing the same handler stack.
type Server struct {
	server    *dns.Server
	tcpServer *dns.Server
	cache     *dnscache.DNSCache
	forwarder *forward.Forwarder
	log       *slog.Logger
}

// New constructs a Server that listens on addr and forwards to upstreams.
func New(addr string, upstreams []string, log *slog.Logger, filter *filtering.Filter) *Server {
	cache := dnscache.New(log)
	forwarder := forward.New(upstreams, log)

	s := &Server{
		server:    &dns.Server{Addr: addr, Net: "udp", UDPSize: 65535},
		tcpServer: &dns.Server{Addr: addr, Net: "tcp"},
		cache:     cache,
		forwarder: forwarder,
		log:       log,
	}

	dnsHandler := handler.New(cache, forwarder, addr, log, filter)
	dns.HandleFunc(".", dnsHandler.HandleDNSRequest)

	return s
}

// Start launches both TCP and UDP listeners and blocks until the UDP server stops.
func (s *Server) Start() error {
	s.log.Info("starting DNS server", "version", version.ResolvitVersion, "address", s.server.Addr)
	go func() {
		if err := s.tcpServer.ListenAndServe(); err != nil {
			s.log.Error("TCP server error", "error", err)
		}
	}()

	if err := s.server.ListenAndServe(); err != nil {
		return err
	}
	return nil
}

// ClearCache removes all cached DNS entries.
func (s *Server) ClearCache() {
	s.cache.Clear()
}
