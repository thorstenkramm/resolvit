package handler

import (
	"fmt"
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
