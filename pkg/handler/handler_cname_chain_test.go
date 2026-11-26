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
	"time"

	"github.com/miekg/dns"
)

// loopGuard is how long a purely local answer may take before the test binary is aborted.
const loopGuard = 500 * time.Millisecond

// queryGuarded sends one question to the handler and returns the reply.
// A handler that is stuck in a CNAME loop cannot be stopped and allocates about 1 GB per second.
// Failing only the test would leave it running, so the whole test binary is aborted instead.
func queryGuarded(t *testing.T, h *Handler, name string, qtype uint16) *dns.Msg {
	t.Helper()

	req := new(dns.Msg)
	req.SetQuestion(name, qtype)
	w := &testResponseWriter{}

	done := make(chan struct{})
	go func() {
		defer close(done)
		h.HandleDNSRequest(w, req)
	}()

	select {
	case <-done:
	case <-time.After(loopGuard):
		panic(fmt.Sprintf("no answer for %s after %s, the handler is stuck in a CNAME loop", name, loopGuard))
	}

	if w.msg == nil {
		t.Fatal("no response message received")
	}
	return w.msg
}

func addCNAMELoopRecords() {
	records.Add("chain-self.example.com.", records.CNAME, "chain-self.example.com")
	records.Add("chain-loop-a.example.com.", records.CNAME, "chain-loop-b.example.com")
	records.Add("chain-loop-b.example.com.", records.CNAME, "chain-loop-a.example.com")
	records.Add("*.chain-wild.example.com.", records.CNAME, "x.chain-wild.example.com")

	// 17 CNAME records in a row, one more than records.MaxCNAMEChain
	for i := 0; i < records.MaxCNAMEChain; i++ {
		records.Add(
			fmt.Sprintf("chain-long%d.example.com.", i),
			records.CNAME,
			fmt.Sprintf("chain-long%d.example.com", i+1),
		)
	}
	records.Add(fmt.Sprintf("chain-long%d.example.com.", records.MaxCNAMEChain), records.CNAME, "chain-long-final.example.com")
	records.Add("chain-long-final.example.com.", records.A, "192.0.2.30")
}

// TestCNAMELoopAnswersServfail verifies that a CNAME loop or an overlong chain in the local
// records is answered with SERVFAIL, and that this answer is not cached.
func TestCNAMELoopAnswersServfail(t *testing.T) {
	addCNAMELoopRecords()

	stub := testutil.StartDNSStub(t, testutil.FixedHandler(map[string]testutil.Response{}))
	log := logger.Setup("debug", "stdout")
	cache := dnscache.New(log)
	h := New(cache, forward.New([]string{stub.Addr}, log), log, nil)

	queries := []struct {
		name  string
		query string
	}{
		{"name pointing to itself", "chain-self.example.com."},
		{"two names pointing to each other", "chain-loop-a.example.com."},
		{"wildcard matching its own target", "foo.chain-wild.example.com."},
		{"chain longer than the limit", "chain-long0.example.com."},
	}

	for _, tt := range queries {
		for _, qtype := range []uint16{dns.TypeA, dns.TypeAAAA} {
			t.Run(tt.name+", "+dns.TypeToString[qtype], func(t *testing.T) {
				msg := queryGuarded(t, h, tt.query, qtype)

				if msg.Rcode != dns.RcodeServerFailure {
					t.Errorf("got rcode %s, want SERVFAIL", dns.RcodeToString[msg.Rcode])
				}
				if len(msg.Answer) != 0 {
					t.Errorf("got %d answers %v, want none", len(msg.Answer), msg.Answer)
				}
				if _, found := cache.Get(cacheKeyFor(tt.query, qtype, "udp")); found {
					t.Error("SERVFAIL answer was cached")
				}
			})
		}
	}
}

// TestCNAMEQuestionForLoopingName verifies that the CNAME record of a looping name can still be
// queried. A CNAME question is answered by the record itself, so the loop is never entered.
func TestCNAMEQuestionForLoopingName(t *testing.T) {
	addCNAMELoopRecords()

	stub := testutil.StartDNSStub(t, testutil.FixedHandler(map[string]testutil.Response{}))
	log := logger.Setup("debug", "stdout")
	h := New(dnscache.New(log), forward.New([]string{stub.Addr}, log), log, nil)

	msg := queryGuarded(t, h, "chain-loop-a.example.com.", dns.TypeCNAME)

	if msg.Rcode != dns.RcodeSuccess {
		t.Errorf("got rcode %s, want NOERROR", dns.RcodeToString[msg.Rcode])
	}
	if len(msg.Answer) != 1 {
		t.Fatalf("got %d answers %v, want the CNAME record only", len(msg.Answer), msg.Answer)
	}
	cname, ok := msg.Answer[0].(*dns.CNAME)
	if !ok {
		t.Fatalf("got answer of type %T, want CNAME", msg.Answer[0])
	}
	if cname.Target != "chain-loop-b.example.com." {
		t.Errorf("got target %q, want chain-loop-b.example.com.", cname.Target)
	}
}

// TestCNAMEUpstreamResult verifies that the upstream result for the last name of a local CNAME
// chain reaches the client: the response code and the authority section are passed through,
// and the local CNAME records stay in the answer section.
func TestCNAMEUpstreamResult(t *testing.T) {
	records.Add("rc-missing.example.com.", records.CNAME, "missing.example.net")
	records.Add("rc-missing-first.example.com.", records.CNAME, "rc-missing.example.com")
	records.Add("rc-broken.example.com.", records.CNAME, "broken.example.net")
	records.Add("rc-v4only.example.com.", records.CNAME, "v4only.example.net")

	soa := testutil.SOARecord("example.net.", 30)
	responses := map[string]testutil.Response{
		testutil.Key("missing.example.net.", dns.TypeA): {Rcode: dns.RcodeNameError, Ns: []dns.RR{soa}},
		testutil.Key("broken.example.net.", dns.TypeA):  {Rcode: dns.RcodeServerFailure},
		testutil.Key("v4only.example.net.", dns.TypeA): {
			Answers: []dns.RR{testutil.ARecord("v4only.example.net.", "198.51.100.2")},
		},
		// Name exists, but has no AAAA record: NOERROR with the SOA record only
		testutil.Key("v4only.example.net.", dns.TypeAAAA): {Ns: []dns.RR{soa}},
	}
	stub := testutil.StartDNSStub(t, testutil.FixedHandler(responses))

	log := logger.Setup("debug", "stdout")
	cache := dnscache.New(log)
	h := New(cache, forward.New([]string{stub.Addr}, log), log, nil)

	tests := []struct {
		name       string
		query      string
		qtype      uint16
		wantRcode  int
		wantTypes  []uint16
		wantSOA    bool
		wantCached bool
	}{
		{"target does not exist", "rc-missing.example.com.", dns.TypeA,
			dns.RcodeNameError, []uint16{dns.TypeCNAME}, true, true},
		{"two local CNAMEs, target does not exist", "rc-missing-first.example.com.", dns.TypeA,
			dns.RcodeNameError, []uint16{dns.TypeCNAME, dns.TypeCNAME}, true, true},
		{"upstream fails for the target", "rc-broken.example.com.", dns.TypeA,
			dns.RcodeServerFailure, []uint16{dns.TypeCNAME}, false, false},
		{"target has no record of the type", "rc-v4only.example.com.", dns.TypeAAAA,
			dns.RcodeSuccess, []uint16{dns.TypeCNAME}, true, true},
		{"target has a record (regression)", "rc-v4only.example.com.", dns.TypeA,
			dns.RcodeSuccess, []uint16{dns.TypeCNAME, dns.TypeA}, false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := new(dns.Msg)
			req.SetQuestion(tt.query, tt.qtype)
			w := &testResponseWriter{}
			h.HandleDNSRequest(w, req)

			if w.msg == nil {
				t.Fatal("no response message received")
			}
			if w.msg.Rcode != tt.wantRcode {
				t.Errorf("got rcode %s, want %s", dns.RcodeToString[w.msg.Rcode], dns.RcodeToString[tt.wantRcode])
			}
			if len(w.msg.Answer) != len(tt.wantTypes) {
				t.Fatalf("got %d answers %v, want types %v", len(w.msg.Answer), w.msg.Answer, tt.wantTypes)
			}
			for i, rr := range w.msg.Answer {
				if rr.Header().Rrtype != tt.wantTypes[i] {
					t.Errorf("answer %d: got type %s, want %s", i,
						dns.TypeToString[rr.Header().Rrtype], dns.TypeToString[tt.wantTypes[i]])
				}
			}

			wantNs := 0
			if tt.wantSOA {
				wantNs = 1
			}
			if len(w.msg.Ns) != wantNs {
				t.Fatalf("got %d authority records %v, want %d", len(w.msg.Ns), w.msg.Ns, wantNs)
			}
			if tt.wantSOA {
				if _, ok := w.msg.Ns[0].(*dns.SOA); !ok {
					t.Errorf("got authority record of type %T, want SOA", w.msg.Ns[0])
				}
			}
			if len(w.msg.Extra) != 0 {
				t.Errorf("got %d additional records %v, want none", len(w.msg.Extra), w.msg.Extra)
			}

			// Only NOERROR and NXDOMAIN may be stored, a failure must be asked again
			if _, found := cache.Get(cacheKeyFor(tt.query, tt.qtype, "udp")); found != tt.wantCached {
				t.Errorf("got cached %v, want %v", found, tt.wantCached)
			}
		})
	}
}

// TestCNAMEUpstreamUnreachable verifies that a failed upstream exchange for the target of a local
// CNAME is answered with SERVFAIL, like any other question the upstream cannot answer.
func TestCNAMEUpstreamUnreachable(t *testing.T) {
	records.Add("rc-unreachable.example.com.", records.CNAME, "target.example.net")

	// A closed local port fails at once, an unreachable address only after the forwarder's timeout
	conn, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	closedAddr := conn.LocalAddr().String()
	if err := conn.Close(); err != nil {
		t.Fatalf("close udp: %v", err)
	}

	log := logger.Setup("debug", "stdout")
	cache := dnscache.New(log)
	h := New(cache, forward.New([]string{closedAddr}, log), log, nil)

	req := new(dns.Msg)
	req.SetQuestion("rc-unreachable.example.com.", dns.TypeA)
	w := &testResponseWriter{}
	h.HandleDNSRequest(w, req)

	if w.msg == nil {
		t.Fatal("no response message received")
	}
	if w.msg.Rcode != dns.RcodeServerFailure {
		t.Errorf("got rcode %s, want SERVFAIL", dns.RcodeToString[w.msg.Rcode])
	}
	if len(w.msg.Answer) != 0 {
		t.Errorf("got %d answers %v, want none", len(w.msg.Answer), w.msg.Answer)
	}
	if _, found := cache.Get(cacheKeyFor("rc-unreachable.example.com.", dns.TypeA, "udp")); found {
		t.Error("SERVFAIL answer was cached")
	}
}
