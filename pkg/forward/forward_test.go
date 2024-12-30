package forward

import (
	"testing"

	"github.com/miekg/dns"
)

func TestForward(t *testing.T) {
	// Create test forwarder with multiple upstream servers
	f := New([]string{"9.9.9.9:53", "8.8.8.8:53", "1.1.1.1:53"}, nil)

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
	// Test fail over with invalid first server
	f := New([]string{"0.0.0.0:53", "9.9.9.9:53"}, nil)

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
