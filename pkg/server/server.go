package server

import (
	"context"
	"log/slog"
	"resolvit/pkg/dnscache"
	"resolvit/pkg/forward"
	"resolvit/pkg/handler"
	"sync"

	"github.com/miekg/dns"
)

const (
	numWorkers = 1
	queueSize  = 100
)

type Server struct {
	server    *dns.Server
	cache     *dnscache.DNSCache
	forwarder *forward.Forwarder
	requests  chan dnsRequest
	log       *slog.Logger
	wg        sync.WaitGroup
}

type dnsRequest struct {
	w dns.ResponseWriter
	r *dns.Msg
}

func New(addr string, upstreams []string, log *slog.Logger) *Server {
	cache := dnscache.New(log)
	forwarder := forward.New(upstreams, log)
	dnsHandler := handler.New(cache, forwarder, addr, log)

	s := &Server{
		server:    &dns.Server{Addr: addr, Net: "udp"},
		cache:     cache,
		forwarder: forwarder,
		requests:  make(chan dnsRequest, queueSize),
		log:       log,
	}

	dns.HandleFunc(".", dnsHandler.HandleDNSRequest)
	return s
}

func (s *Server) handleRequest(w dns.ResponseWriter, r *dns.Msg) {
	select {
	case s.requests <- dnsRequest{w, r}:
		// Request queued successfully
	default:
		s.log.Error("request queue full, dropping request")
		m := new(dns.Msg)
		m.SetRcode(r, dns.RcodeServerFailure)
		err := w.WriteMsg(m)
		if err != nil {
			s.log.Error("failed to write DNS response", "error", err)
			return
		}
	}
}

func (s *Server) Start(ctx context.Context) error {
	// Start workers
	for i := 0; i < numWorkers; i++ {
		s.wg.Add(1)
		go s.worker(ctx, i)
	}

	// Start server
	go func() {
		s.log.Info("starting DNS server", "address", s.server.Addr)
		if err := s.server.ListenAndServe(); err != nil {
			s.log.Error("server failed", "error", err)
		}
	}()

	return nil
}

func (s *Server) worker(ctx context.Context, id int) {
	defer s.wg.Done()
	dnsHandler := handler.New(s.cache, s.forwarder, s.server.Addr, s.log)
	for {
		select {
		case req := <-s.requests:
			dnsHandler.HandleDNSRequest(req.w, req.r)
		case <-ctx.Done():
			s.log.Debug("worker shutting down", "id", id)
			return
		}
	}
}

func (s *Server) Shutdown(ctx context.Context) error {
	// Stop accepting new requests
	if err := s.server.ShutdownContext(ctx); err != nil {
		return err
	}

	// Wait for workers to finish
	s.wg.Wait()
	s.log.Info("server shutdown complete")
	return nil
}
