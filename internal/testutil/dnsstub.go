// Package testutil provides helpers for deterministic DNS tests.
package testutil

import (
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/miekg/dns"
)

// Response defines a fixed DNS reply for a question.
type Response struct {
	Answers []dns.RR
	Rcode   int
}

// DNSStub hosts UDP and TCP DNS servers on the same port.
type DNSStub struct {
	Addr      string
	udpServer *dns.Server
	tcpServer *dns.Server
}

// StartDNSStub starts a DNS server for both UDP and TCP on a random port.
func StartDNSStub(t *testing.T, handler dns.Handler) *DNSStub {
	t.Helper()

	udpConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}

	udpAddr := udpConn.LocalAddr().(*net.UDPAddr)
	addr := fmt.Sprintf("127.0.0.1:%d", udpAddr.Port)

	tcpListener, err := net.Listen("tcp", addr)
	if err != nil {
		_ = udpConn.Close()
		t.Fatalf("listen tcp: %v", err)
	}

	stub := &DNSStub{
		Addr:      addr,
		udpServer: &dns.Server{PacketConn: udpConn, Handler: handler},
		tcpServer: &dns.Server{Listener: tcpListener, Handler: handler},
	}

	errCh := make(chan error, 2)
	go func() { errCh <- stub.udpServer.ActivateAndServe() }()
	go func() { errCh <- stub.tcpServer.ActivateAndServe() }()

	if err := waitForTCP(addr); err != nil {
		stub.Close()
		t.Fatalf("wait for dns stub: %v", err)
	}

	t.Cleanup(func() {
		stub.Close()
		select {
		case <-errCh:
		default:
		}
	})

	return stub
}

// Close shuts down the DNS stub servers.
func (s *DNSStub) Close() {
	if s.tcpServer != nil {
		_ = s.tcpServer.Shutdown()
	}
	if s.udpServer != nil {
		_ = s.udpServer.Shutdown()
	}
}

// FixedHandler returns a handler that serves fixed responses by question.
func FixedHandler(responses map[string]Response) dns.Handler {
	return dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		reply := new(dns.Msg)
		if r == nil || len(r.Question) == 0 {
			reply.Rcode = dns.RcodeFormatError
			_ = w.WriteMsg(reply)
			return
		}

		reply.SetReply(r)
		reply.RecursionAvailable = true
		q := r.Question[0]
		if resp, ok := responses[Key(q.Name, q.Qtype)]; ok {
			if resp.Rcode != 0 {
				reply.Rcode = resp.Rcode
			}
			reply.Answer = append(reply.Answer, resp.Answers...)
		} else {
			reply.Rcode = dns.RcodeNameError
		}

		_ = w.WriteMsg(reply)
	})
}

// Key returns a canonical map key for a question.
func Key(name string, qtype uint16) string {
	return strings.ToLower(NormalizeName(name)) + "|" + strconv.Itoa(int(qtype))
}

// NormalizeName lowercases and ensures a trailing dot for DNS names.
func NormalizeName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	lower := strings.ToLower(trimmed)
	if strings.HasSuffix(lower, ".") {
		return lower
	}
	return lower + "."
}

// ARecord creates an A record with a standard TTL.
func ARecord(name string, ip string) dns.RR {
	return &dns.A{
		Hdr: dns.RR_Header{
			Name:   NormalizeName(name),
			Rrtype: dns.TypeA,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		A: net.ParseIP(ip),
	}
}

// AAAARecord creates an AAAA record with a standard TTL.
func AAAARecord(name string, ip string) dns.RR {
	return &dns.AAAA{
		Hdr: dns.RR_Header{
			Name:   NormalizeName(name),
			Rrtype: dns.TypeAAAA,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		AAAA: net.ParseIP(ip),
	}
}

// CNAMERecord creates a CNAME record with a standard TTL.
func CNAMERecord(name string, target string) dns.RR {
	return &dns.CNAME{
		Hdr: dns.RR_Header{
			Name:   NormalizeName(name),
			Rrtype: dns.TypeCNAME,
			Class:  dns.ClassINET,
			Ttl:    60,
		},
		Target: NormalizeName(target),
	}
}

func waitForTCP(addr string) error {
	var lastErr error
	for i := 0; i < 20; i++ {
		conn, err := net.DialTimeout("tcp", addr, 50*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		lastErr = err
		time.Sleep(10 * time.Millisecond)
	}
	return lastErr
}
