package handler

import (
	"fmt"
	"net"
	"resolvit/internal/testutil"
	"resolvit/pkg/dnscache"
	"resolvit/pkg/forward"
	"resolvit/pkg/logger"
	"resolvit/pkg/records"
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
			wantTarget: "", // Don't check specific IP as it may change (CDN)
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

	responses := map[string]testutil.Response{
		testutil.Key("example.com.", dns.TypeA): {
			Answers: []dns.RR{testutil.ARecord("example.com.", "93.184.216.34")},
		},
		testutil.Key("cname-localhost.sys25.net.", dns.TypeA): {
			Answers: []dns.RR{
				testutil.CNAMERecord("cname-localhost.sys25.net.", "localhost."),
				testutil.ARecord("localhost.", "127.0.0.1"),
			},
		},
		testutil.Key("monitoring.hcloud.dimedis.net.", dns.TypeAAAA): {
			Answers: []dns.RR{testutil.AAAARecord("monitoring.hcloud.dimedis.net.", "2a01:4f9:c010:cf73::1")},
		},
	}
	stub := testutil.StartDNSStub(t, testutil.FixedHandler(responses))

	logger := logger.Setup("debug", "stdout")
	cache := dnscache.New(logger)
	forwarder := forward.New([]string{stub.Addr}, logger)
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

			// Only check target if expected target is specified
			if tt.wantTarget != "" && target != tt.wantTarget {
				t.Errorf("Got target %s, want %s", target, tt.wantTarget)
			}

			// Test cache
			if tt.wantCache {
				// Cache key includes protocol - default is UDP
				protocol := "udp"
				if w.network == "tcp" {
					protocol = "tcp"
				}
				cacheKey := cacheKeyFor(tt.query, dns.TypeA, protocol)
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

	responses := map[string]testutil.Response{
		testutil.Key("unused.example.com.", dns.TypeA): {
			Answers: []dns.RR{testutil.ARecord("unused.example.com.", "192.0.2.1")},
		},
	}
	stub := testutil.StartDNSStub(t, testutil.FixedHandler(responses))

	logger := logger.Setup("debug", "stdout")
	cache := dnscache.New(logger)
	forwarder := forward.New([]string{stub.Addr}, logger)
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
	msg     *dns.Msg
	network string // "tcp" or "udp"
}

func (w *testResponseWriter) LocalAddr() net.Addr {
	if w.network == "tcp" {
		return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 53}
	}
	return &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 53}
}

func (w *testResponseWriter) RemoteAddr() net.Addr {
	if w.network == "tcp" {
		return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 1053}
	}
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

// TestTCPvsUDPTruncation verifies that UDP responses get truncated but TCP responses remain complete
func TestTCPvsUDPTruncation(t *testing.T) {
	// Add final A record target
	records.Add("final-tcp-test.example.com.", records.A, "192.168.1.20")

	// Create a long chain of CNAME records (10 CNAMEs)
	for i := 0; i < 10; i++ {
		records.Add(
			fmt.Sprintf("tcptest-cname%d.example.com.", i),
			records.CNAME,
			fmt.Sprintf("tcptest-cname%d.example.com", i+1),
		)
	}
	// Final CNAME points to our local A record
	records.Add("tcptest-cname10.example.com.", records.CNAME, "final-tcp-test.example.com")

	responses := map[string]testutil.Response{
		testutil.Key("unused.example.com.", dns.TypeA): {
			Answers: []dns.RR{testutil.ARecord("unused.example.com.", "192.0.2.1")},
		},
	}
	stub := testutil.StartDNSStub(t, testutil.FixedHandler(responses))

	logger := logger.Setup("debug", "stdout")
	cache := dnscache.New(logger)
	forwarder := forward.New([]string{stub.Addr}, logger)
	h := New(cache, forwarder, "127.0.0.1:5300", logger)

	req := new(dns.Msg)
	req.SetQuestion("tcptest-cname0.example.com.", dns.TypeA)
	req.RecursionDesired = true

	// Test UDP response
	t.Run("UDP gets truncated", func(t *testing.T) {
		wUDP := &testResponseWriter{network: "udp"}
		h.HandleDNSRequest(wUDP, req)

		if wUDP.msg == nil {
			t.Fatal("No response message received")
		}

		// Verify truncation bit is set for UDP
		if !wUDP.msg.Truncated {
			t.Error("Expected UDP message to be truncated")
		}

		// Verify message size is within UDP limits
		if wUDP.msg.Len() > dns.DefaultMsgSize {
			t.Errorf("UDP message size %d exceeds default size %d", wUDP.msg.Len(), dns.DefaultMsgSize)
		}

		t.Logf("UDP response contains %d answers", len(wUDP.msg.Answer))
		t.Logf("UDP message size: %d bytes", wUDP.msg.Len())
	})

	// Test TCP response
	t.Run("TCP sends complete response", func(t *testing.T) {
		wTCP := &testResponseWriter{network: "tcp"}
		h.HandleDNSRequest(wTCP, req)

		if wTCP.msg == nil {
			t.Fatal("No response message received")
		}

		// Verify truncation bit is NOT set for TCP
		if wTCP.msg.Truncated {
			t.Error("Expected TCP message to NOT be truncated")
		}

		// Verify we have all records (11 CNAMEs + 1 A record = 12 total)
		expectedAnswers := 12
		if len(wTCP.msg.Answer) != expectedAnswers {
			t.Errorf("Expected %d answers in TCP response, got %d", expectedAnswers, len(wTCP.msg.Answer))
		}

		// Verify the final record is an A record with the correct IP
		lastAnswer := wTCP.msg.Answer[len(wTCP.msg.Answer)-1]
		aRecord, ok := lastAnswer.(*dns.A)
		if !ok {
			t.Fatalf("Last answer is not an A record, got %T", lastAnswer)
		}

		expectedIP := "192.168.1.20"
		if aRecord.A.String() != expectedIP {
			t.Errorf("Expected final A record to be %s, got %s", expectedIP, aRecord.A.String())
		}

		t.Logf("TCP response contains %d answers", len(wTCP.msg.Answer))
		t.Logf("TCP message size: %d bytes", wTCP.msg.Len())
		t.Logf("Final A record: %s", aRecord.A.String())
	})
}

// TestSeparateUDPTCPCache verifies that UDP and TCP responses are cached separately
func TestSeparateUDPTCPCache(t *testing.T) {
	// Add final A record target
	records.Add("cache-test-final.example.com.", records.A, "192.168.1.30")

	// Create a long chain of CNAME records
	for i := 0; i < 10; i++ {
		records.Add(
			fmt.Sprintf("cachetest-cname%d.example.com.", i),
			records.CNAME,
			fmt.Sprintf("cachetest-cname%d.example.com", i+1),
		)
	}
	records.Add("cachetest-cname10.example.com.", records.CNAME, "cache-test-final.example.com")

	responses := map[string]testutil.Response{
		testutil.Key("unused.example.com.", dns.TypeA): {
			Answers: []dns.RR{testutil.ARecord("unused.example.com.", "192.0.2.1")},
		},
	}
	stub := testutil.StartDNSStub(t, testutil.FixedHandler(responses))

	logger := logger.Setup("debug", "stdout")
	cache := dnscache.New(logger)
	forwarder := forward.New([]string{stub.Addr}, logger)
	h := New(cache, forwarder, "127.0.0.1:5300", logger)

	req := new(dns.Msg)
	req.SetQuestion("cachetest-cname0.example.com.", dns.TypeA)
	req.RecursionDesired = true

	// First request via UDP - will be cached as truncated
	wUDP := &testResponseWriter{network: "udp"}
	h.HandleDNSRequest(wUDP, req)

	if wUDP.msg == nil {
		t.Fatal("No UDP response message received")
	}

	udpAnswerCount := len(wUDP.msg.Answer)
	t.Logf("UDP cached response has %d answers", udpAnswerCount)

	// Second request via TCP - should have separate cache entry with full response
	req2 := new(dns.Msg)
	req2.SetQuestion("cachetest-cname0.example.com.", dns.TypeA)
	req2.RecursionDesired = true

	wTCP := &testResponseWriter{network: "tcp"}
	h.HandleDNSRequest(wTCP, req2)

	if wTCP.msg == nil {
		t.Fatal("No TCP response message received")
	}

	tcpAnswerCount := len(wTCP.msg.Answer)
	t.Logf("TCP cached response has %d answers", tcpAnswerCount)

	// Verify TCP has more answers than UDP (complete vs truncated)
	if tcpAnswerCount <= udpAnswerCount {
		t.Errorf("Expected TCP response (%d answers) to have more answers than UDP response (%d answers)",
			tcpAnswerCount, udpAnswerCount)
	}

	// Verify TCP has the final A record
	lastAnswer := wTCP.msg.Answer[len(wTCP.msg.Answer)-1]
	if _, ok := lastAnswer.(*dns.A); !ok {
		t.Errorf("TCP response missing final A record, last answer is %T", lastAnswer)
	}

	// Third request via TCP - should come from cache and still be complete
	req3 := new(dns.Msg)
	req3.SetQuestion("cachetest-cname0.example.com.", dns.TypeA)
	req3.RecursionDesired = true

	wTCP2 := &testResponseWriter{network: "tcp"}
	h.HandleDNSRequest(wTCP2, req3)

	if wTCP2.msg == nil {
		t.Fatal("No second TCP response message received")
	}

	// Verify the cached TCP response is still complete
	if len(wTCP2.msg.Answer) != tcpAnswerCount {
		t.Errorf("Cached TCP response has %d answers, expected %d", len(wTCP2.msg.Answer), tcpAnswerCount)
	}

	t.Log("Verified: UDP and TCP maintain separate cache entries")
}

// TestFinalARecordPreservedInTCP verifies that the final A record is never lost in TCP responses
func TestFinalARecordPreservedInTCP(t *testing.T) {
	// Add final A record target with specific IP
	targetIP := "10.20.30.40"
	records.Add("critical-target.example.com.", records.A, targetIP)

	// Create an even longer chain (15 CNAMEs) to ensure we exceed UDP limits
	for i := 0; i < 15; i++ {
		records.Add(
			fmt.Sprintf("long-cname%d.example.com.", i),
			records.CNAME,
			fmt.Sprintf("long-cname%d.example.com", i+1),
		)
	}
	records.Add("long-cname15.example.com.", records.CNAME, "critical-target.example.com")

	responses := map[string]testutil.Response{
		testutil.Key("unused.example.com.", dns.TypeA): {
			Answers: []dns.RR{testutil.ARecord("unused.example.com.", "192.0.2.1")},
		},
	}
	stub := testutil.StartDNSStub(t, testutil.FixedHandler(responses))

	logger := logger.Setup("debug", "stdout")
	cache := dnscache.New(logger)
	forwarder := forward.New([]string{stub.Addr}, logger)
	h := New(cache, forwarder, "127.0.0.1:5300", logger)

	req := new(dns.Msg)
	req.SetQuestion("long-cname0.example.com.", dns.TypeA)
	req.RecursionDesired = true

	// Test via TCP
	wTCP := &testResponseWriter{network: "tcp"}
	h.HandleDNSRequest(wTCP, req)

	if wTCP.msg == nil {
		t.Fatal("No response message received")
	}

	if len(wTCP.msg.Answer) == 0 {
		t.Fatal("TCP response has no answers")
	}

	// The critical test: verify the last answer is the A record with our target IP
	lastAnswer := wTCP.msg.Answer[len(wTCP.msg.Answer)-1]
	aRecord, ok := lastAnswer.(*dns.A)
	if !ok {
		t.Fatalf("CRITICAL BUG: Final answer is not an A record, got %T. This is the bug we fixed!", lastAnswer)
	}

	if aRecord.A.String() != targetIP {
		t.Errorf("CRITICAL BUG: Final A record has IP %s, expected %s", aRecord.A.String(), targetIP)
	}

	// Verify we have all 17 records (16 CNAMEs + 1 A record)
	expectedAnswers := 17
	if len(wTCP.msg.Answer) != expectedAnswers {
		t.Errorf("Expected %d answers in complete chain, got %d", expectedAnswers, len(wTCP.msg.Answer))
	}

	t.Logf("SUCCESS: TCP preserved all %d records including final A record: %s",
		len(wTCP.msg.Answer), aRecord.A.String())
	t.Logf("Message size: %d bytes", wTCP.msg.Len())
}
