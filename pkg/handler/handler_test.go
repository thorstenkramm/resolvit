package handler

import (
	"fmt"
	"net"
	"resolvit/pkg/dnscache"
	"resolvit/pkg/forward"
	"resolvit/pkg/logger"
	"resolvit/pkg/records"
	"strconv"
	"testing"

	"github.com/miekg/dns"
)

func TestHandleDNSRequest(t *testing.T) {
	records.Add("local.example.com.", records.A, "192.168.1.10")
	records.Add("*.wildcard.example.com.", records.A, "192.168.1.11")
	records.Add("alias.example.com.", records.CNAME, "local.example.com")
	records.Add("*.wildcardalias.example.com.", records.CNAME, "local.example.com")
	records.Add("cname1.example.com.", records.CNAME, "cname2.example.com")
	records.Add("cname2.example.com.", records.CNAME, "cname3.example.com")
	records.Add("cname3.example.com.", records.CNAME, "cname-localhost.sys25.net")

	t.Logf("Having %d local record(s)", len(records.GetAll()))

	tests := []struct {
		name       string
		query      string
		wantType   uint16
		wantTarget string
		wantCache  bool
		wantAuth   bool
		wantRA     bool
	}{
		{
			name:       "Local A record",
			query:      "local.example.com.",
			wantType:   dns.TypeA,
			wantTarget: "192.168.1.10",
			wantCache:  true,
			wantAuth:   true,
			wantRA:     true,
		},
		{
			name:       "Local CNAME record",
			query:      "alias.example.com.",
			wantType:   dns.TypeCNAME,
			wantTarget: "192.168.1.10",
			wantCache:  true,
			wantAuth:   true,
			wantRA:     true,
		},
		{
			name:       "Local A wildcard record",
			query:      "foo.wildcard.example.com.",
			wantType:   dns.TypeA,
			wantTarget: "192.168.1.11",
			wantCache:  true,
			wantAuth:   true,
			wantRA:     true,
		},
		{
			name:       "Local CNAME wildcard record",
			query:      "foo.wildcardalias.example.com.",
			wantType:   dns.TypeCNAME,
			wantTarget: "192.168.1.10",
			wantCache:  true,
			wantAuth:   true,
			wantRA:     true,
		},
		{
			name:       "Remote A record",
			query:      "example.com.",
			wantType:   dns.TypeA,
			wantTarget: "93.184.215.14",
			wantCache:  true,
			wantAuth:   false,
			wantRA:     true,
		},
		{
			name:       "Remote CNAME record",
			query:      "cname-localhost.sys25.net.",
			wantType:   dns.TypeCNAME,
			wantTarget: "127.0.0.1",
			wantCache:  true,
			wantAuth:   false,
			wantRA:     true,
		},
		{
			name:       "Nested CNAME records",
			query:      "cname1.example.com.",
			wantType:   dns.TypeCNAME,
			wantTarget: "127.0.0.1",
			wantRA:     true,
		},
		{
			name:       "Local CNAME with external target",
			query:      "cname3.example.com.",
			wantType:   dns.TypeCNAME,
			wantTarget: "127.0.0.1",
			wantRA:     true,
		},
		{
			name:       "Remote AAAA record",
			query:      "monitoring.hcloud.dimedis.net.",
			wantType:   dns.TypeAAAA,
			wantTarget: "2a01:4f9:c010:cf73::1",
		},
	}

	logger := logger.Setup("debug", "stdout")
	cache := dnscache.New(logger)
	forwarder := forward.New([]string{"8.8.8.8:53"}, logger)
	// Create a new DNS Request handler
	h := New(cache, forwarder, "127.0.0.1:5300", logger)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := new(dns.Msg)

			qType := dns.TypeA
			if net.ParseIP(tt.wantTarget) != nil && net.ParseIP(tt.wantTarget).To4() == nil {
				// Target is ip v6
				qType = dns.TypeAAAA
			}
			req.SetQuestion(tt.query, qType)

			w := &testResponseWriter{}
			h.HandleDNSRequest(w, req)

			if w.msg == nil {
				t.Fatal("No response message received")
			}

			if len(w.msg.Answer) == 0 {
				t.Fatal("No answer section in response")
			}

			answer := w.msg.Answer[0]
			if answer.Header().Rrtype != tt.wantType {
				t.Errorf("Got record type %d, want %d", answer.Header().Rrtype, tt.wantType)
			}

			for i, a := range w.msg.Answer {
				t.Logf("Got answer %d: %s", i, a.String())
			}

			var target string
			if qType == dns.TypeAAAA {
				target = w.msg.Answer[len(w.msg.Answer)-1].(*dns.AAAA).AAAA.String()
			} else {
				target = w.msg.Answer[len(w.msg.Answer)-1].(*dns.A).A.String()
			}
			t.Logf("Got Target %s", target)

			if target != tt.wantTarget {
				t.Errorf("Got target %s, want %s", target, tt.wantTarget)
			}

			// Test cache
			if tt.wantCache {
				cacheKey := tt.query + strconv.Itoa(int(dns.TypeA))
				_, found := cache.Get(cacheKey)
				if !found {
					t.Error("Expected response to be cached")
				}
			}

			// Validate response flags
			if w.msg.Authoritative != tt.wantAuth {
				t.Errorf("Got Authoritative=%v, want %v", w.msg.Authoritative, tt.wantAuth)
			}
			if w.msg.RecursionAvailable != true {
				t.Errorf("Got RecursionAvailable=%v, want %v", w.msg.RecursionAvailable, true)
			}
		})
	}
}

func TestMessageTruncation(t *testing.T) {
	// Add final A record target
	records.Add("final.example.com.", records.A, "192.168.1.10")

	// Create a long chain of CNAME records
	for i := 0; i < 10; i++ {
		records.Add(
			fmt.Sprintf("cname%d.example.com.", i),
			records.CNAME,
			fmt.Sprintf("cname%d.example.com", i+1),
		)
	}
	// Final CNAME points to our local A record
	records.Add("cname11.example.com.", records.CNAME, "final.example.com")

	logger := logger.Setup("debug", "stdout")
	cache := dnscache.New(logger)
	forwarder := forward.New([]string{"8.8.8.8:53"}, logger)
	h := New(cache, forwarder, "127.0.0.1:5300", logger)

	req := new(dns.Msg)
	req.SetQuestion("cname0.example.com.", dns.TypeA)
	req.RecursionDesired = true

	w := &testResponseWriter{}
	h.HandleDNSRequest(w, req)

	if w.msg == nil {
		t.Fatal("No response message received")
	}

	// Verify truncation bit is set
	if !w.msg.Truncated {
		t.Error("Expected message to be truncated")
	}

	// Verify message size is within limits
	if w.msg.Len() > dns.DefaultMsgSize {
		t.Errorf("Message size %d exceeds default size %d", w.msg.Len(), dns.DefaultMsgSize)
	}

	t.Logf("Response contains %d answers", len(w.msg.Answer))
	t.Logf("Message size: %d bytes", w.msg.Len())

	for i, a := range w.msg.Answer {
		t.Logf("Got answer %d: %s", i, a.String())
	}
}

// Mock DNS response writer for testing
type testResponseWriter struct {
	msg *dns.Msg
}

func (w *testResponseWriter) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 53}
}

func (w *testResponseWriter) RemoteAddr() net.Addr {
	return &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1053}
}

func (w *testResponseWriter) WriteMsg(msg *dns.Msg) error {
	w.msg = msg
	return nil
}

func (w *testResponseWriter) Write([]byte) (int, error) {
	return 0, nil
}

func (w *testResponseWriter) Close() error {
	return nil
}

func (w *testResponseWriter) TsigStatus() error {
	return nil
}

func (w *testResponseWriter) TsigTimersOnly(bool) {
}

func (w *testResponseWriter) Hijack() {
}
