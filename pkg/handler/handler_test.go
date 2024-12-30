package handler

import (
	"log/slog"
	"net"
	"resolvit/pkg/dnscache"
	"resolvit/pkg/forward"
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

	t.Logf("Having %d local record(s)", len(records.GetAll()))

	tests := []struct {
		name       string
		query      string
		queryType  uint16
		wantType   uint16
		wantAnswer string
		wantCache  bool
		wantAuth   bool
		wantRA     bool
	}{
		{
			name:       "Local A record",
			query:      "local.example.com.",
			queryType:  dns.TypeA,
			wantType:   dns.TypeA,
			wantAnswer: "192.168.1.10",
			wantCache:  true,
			wantAuth:   true,
			wantRA:     true,
		},
		{
			name:       "Local CNAME record",
			query:      "alias.example.com.",
			queryType:  dns.TypeA,
			wantType:   dns.TypeCNAME,
			wantAnswer: "local.example.com.",
			wantCache:  true,
			wantAuth:   false,
			wantRA:     true,
		},
		{
			name:       "Local A wildcard record",
			query:      "foo.wildcard.example.com.",
			queryType:  dns.TypeA,
			wantType:   dns.TypeA,
			wantAnswer: "192.168.1.11",
			wantCache:  true,
			wantAuth:   true,
			wantRA:     true,
		},
		{
			name:       "Local CNAME wildcard record",
			query:      "foo.wildcardalias.example.com.",
			queryType:  dns.TypeA,
			wantType:   dns.TypeCNAME,
			wantAnswer: "local.example.com.",
			wantCache:  true,
			wantAuth:   false,
			wantRA:     true,
		},
		{
			name:       "Remote A record",
			query:      "example.com.",
			queryType:  dns.TypeA,
			wantType:   dns.TypeA,
			wantAnswer: "93.184.215.14",
			wantCache:  true,
			wantAuth:   false,
			wantRA:     true,
		},
		{
			name:       "Remote CNAME record",
			query:      "www.github.com.",
			queryType:  dns.TypeA,
			wantType:   dns.TypeCNAME,
			wantAnswer: "github.com.",
			wantCache:  true,
			wantAuth:   false,
			wantRA:     true,
		},
	}

	logger := slog.Default()
	cache := dnscache.New(logger)
	forwarder := forward.New([]string{"8.8.8.8:53"}, logger)
	h := New(cache, forwarder, "127.0.0.1:5300", logger)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := new(dns.Msg)
			req.SetQuestion(tt.query, tt.queryType)

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

			switch tt.wantType {
			case dns.TypeA:
				aRecord := answer.(*dns.A)
				if aRecord.A.String() != tt.wantAnswer {
					t.Errorf("Got IP %s, want %s", aRecord.A.String(), tt.wantAnswer)
				}
			case dns.TypeCNAME:
				cnameRecord := answer.(*dns.CNAME)
				if cnameRecord.Target != tt.wantAnswer {
					t.Errorf("Got target %s, want %s", cnameRecord.Target, tt.wantAnswer)
				}
			}

			// Test cache
			if tt.wantCache {
				cacheKey := tt.query + strconv.Itoa(int(tt.queryType))
				_, found := cache.Get(cacheKey)
				if !found {
					t.Error("Expected response to be cached")
				}
			}

			// Validate response flags
			if w.msg.Authoritative != tt.wantAuth {
				t.Errorf("Got Authoritative=%v, want %v", w.msg.Authoritative, tt.wantAuth)
			}
			if w.msg.RecursionAvailable != tt.wantRA {
				t.Errorf("Got RecursionAvailable=%v, want %v", w.msg.RecursionAvailable, tt.wantRA)
			}
		})
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
