package forward

import (
	"log/slog"
	"testing"

	"resolvit/internal/testutil"

	"github.com/miekg/dns"
)

func TestForward(t *testing.T) {
	responses := map[string]testutil.Response{
		testutil.Key("example.com.", dns.TypeA): {
			Answers: []dns.RR{testutil.ARecord("example.com.", "93.184.216.34")},
		},
	}
	stub := testutil.StartDNSStub(t, testutil.FixedHandler(responses))

	// Create test forwarder with the local stub upstream
	f := New([]string{stub.Addr}, slog.Default())

	// Create test DNS message
	m := new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeA)

	// Test forwarding
	resp, err := f.Forward(m)
	if err != nil {
		t.Fatalf("Forward failed: %v", err)
	}

	if resp == nil {
		t.Fatal("Expected response, got nil")
	}

	if len(resp.Answer) == 0 {
		t.Error("Expected answers in response")
	}
}

func TestForwardFailover(t *testing.T) {
	responses := map[string]testutil.Response{
		testutil.Key("example.com.", dns.TypeA): {
			Answers: []dns.RR{testutil.ARecord("example.com.", "93.184.216.34")},
		},
	}
	stub := testutil.StartDNSStub(t, testutil.FixedHandler(responses))

	// Test failover with invalid first server and stub second
	f := New([]string{"0.0.0.0:53", stub.Addr}, slog.Default())

	m := new(dns.Msg)
	m.SetQuestion("example.com.", dns.TypeA)

	resp, err := f.Forward(m)
	if err != nil {
		t.Fatalf("Failover failed: %v", err)
	}

	if resp == nil {
		t.Fatal("Expected response from failover server")
	}
}

func TestForwardRetriesOnTruncation(t *testing.T) {
	handler := dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		reply := new(dns.Msg)
		reply.SetReply(r)
		name := r.Question[0].Name
		if w.RemoteAddr().Network() == "udp" {
			reply.Truncated = true
			reply.Answer = []dns.RR{testutil.ARecord(name, "192.0.2.10")}
			_ = w.WriteMsg(reply)
			return
		}

		reply.Answer = []dns.RR{
			testutil.ARecord(name, "192.0.2.10"),
			testutil.ARecord("extra.example.com.", "192.0.2.11"),
		}
		_ = w.WriteMsg(reply)
	})

	stub := testutil.StartDNSStub(t, handler)
	f := New([]string{stub.Addr}, slog.Default())

	m := new(dns.Msg)
	m.SetQuestion("trunc.example.com.", dns.TypeA)

	resp, err := f.Forward(m)
	if err != nil {
		t.Fatalf("Forward failed: %v", err)
	}
	if resp == nil {
		t.Fatal("Expected response, got nil")
	}
	if resp.Truncated {
		t.Error("Expected full response after TCP retry")
	}
	if len(resp.Answer) < 2 {
		t.Errorf("Expected TCP response with multiple answers, got %d", len(resp.Answer))
	}
}
