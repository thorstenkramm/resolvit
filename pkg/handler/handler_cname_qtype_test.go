package handler

import (
	"resolvit/internal/testutil"
	"resolvit/pkg/dnscache"
	"resolvit/pkg/forward"
	"resolvit/pkg/logger"
	"resolvit/pkg/records"
	"testing"

	"github.com/miekg/dns"
)

// TestCNAMEKeepsQueryType verifies that the type of the original question is kept
// while the CNAME chain is followed. A target without a record of that type must
// yield the CNAME chain only (NOERROR/NODATA), never the target's A record.
func TestCNAMEKeepsQueryType(t *testing.T) {
	records.Add("qt-local-a.example.com.", records.A, "192.0.2.10")
	records.Add("qt-alias-local.example.com.", records.CNAME, "qt-local-a.example.com")
	records.Add("qt-alias-v6.example.com.", records.CNAME, "dualstack.example.net")
	records.Add("qt-alias-v4only.example.com.", records.CNAME, "v4only.example.net")

	responses := map[string]testutil.Response{
		testutil.Key("dualstack.example.net.", dns.TypeA): {
			Answers: []dns.RR{testutil.ARecord("dualstack.example.net.", "198.51.100.1")},
		},
		testutil.Key("dualstack.example.net.", dns.TypeAAAA): {
			Answers: []dns.RR{testutil.AAAARecord("dualstack.example.net.", "2001:db8::1")},
		},
		testutil.Key("v4only.example.net.", dns.TypeA): {
			Answers: []dns.RR{testutil.ARecord("v4only.example.net.", "198.51.100.2")},
		},
		// Name exists, but has no AAAA or MX record: NOERROR with empty answer
		testutil.Key("v4only.example.net.", dns.TypeAAAA): {},
		testutil.Key("v4only.example.net.", dns.TypeMX):   {},
	}
	stub := testutil.StartDNSStub(t, testutil.FixedHandler(responses))

	log := logger.Setup("debug", "stdout")
	h := New(dnscache.New(log), forward.New([]string{stub.Addr}, log), log, nil)

	tests := []struct {
		name      string
		query     string
		qtype     uint16
		wantTypes []uint16
		wantLast  string
	}{
		{"AAAA, external target with AAAA", "qt-alias-v6.example.com.", dns.TypeAAAA,
			[]uint16{dns.TypeCNAME, dns.TypeAAAA}, "2001:db8::1"},
		{"AAAA, external target without AAAA", "qt-alias-v4only.example.com.", dns.TypeAAAA,
			[]uint16{dns.TypeCNAME}, ""},
		{"MX, external target without MX", "qt-alias-v4only.example.com.", dns.TypeMX,
			[]uint16{dns.TypeCNAME}, ""},
		{"AAAA, local A target", "qt-alias-local.example.com.", dns.TypeAAAA,
			[]uint16{dns.TypeCNAME}, ""},
		{"CNAME query returns the CNAME only", "qt-alias-local.example.com.", dns.TypeCNAME,
			[]uint16{dns.TypeCNAME}, ""},
		{"A, local A target (regression)", "qt-alias-local.example.com.", dns.TypeA,
			[]uint16{dns.TypeCNAME, dns.TypeA}, "192.0.2.10"},
		{"A, external target (regression)", "qt-alias-v6.example.com.", dns.TypeA,
			[]uint16{dns.TypeCNAME, dns.TypeA}, "198.51.100.1"},
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
			if w.msg.Rcode != dns.RcodeSuccess {
				t.Errorf("got rcode %s, want NOERROR", dns.RcodeToString[w.msg.Rcode])
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
			if tt.wantLast != "" {
				var got string
				switch rr := w.msg.Answer[len(w.msg.Answer)-1].(type) {
				case *dns.A:
					got = rr.A.String()
				case *dns.AAAA:
					got = rr.AAAA.String()
				}
				if got != tt.wantLast {
					t.Errorf("got last answer %q, want %q", got, tt.wantLast)
				}
			}
		})
	}
}
