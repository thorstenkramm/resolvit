package dnscache

import (
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestDNSCache(t *testing.T) {
	tests := []struct {
		name     string
		msg      *dns.Msg
		key      string
		wait     time.Duration
		wantHit  bool
		wantResp bool
	}{
		{
			name:     "Cache hit - valid TTL",
			msg:      createTestMsg("example.com.", "93.184.216.34", 2),
			key:      "test1",
			wait:     1 * time.Second,
			wantHit:  true,
			wantResp: true,
		},
		{
			name:     "Cache miss - expired TTL",
			msg:      createTestMsg("example.com.", "93.184.216.34", 1),
			key:      "test2",
			wait:     2 * time.Second,
			wantHit:  false,
			wantResp: false,
		},
		{
			name:     "Cache miss - nonexistent key",
			msg:      nil,
			key:      "nonexistent",
			wait:     0,
			wantHit:  false,
			wantResp: false,
		},
	}

	logger := slog.Default()
	cache := New(logger)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.msg != nil {
				cache.Set(tt.key, tt.msg)
				time.Sleep(tt.wait)
			}

			cached, found := cache.Get(tt.key)
			if found != tt.wantHit {
				t.Errorf("cache hit = %v, want %v", found, tt.wantHit)
			}
			if (cached != nil) != tt.wantResp {
				t.Errorf("cached response = %v, want %v", cached != nil, tt.wantResp)
			}
		})
	}
}

func createTestMsg(domain, ip string, ttl uint32) *dns.Msg {
	msg := new(dns.Msg)
	msg.SetQuestion(domain, dns.TypeA)
	msg.Answer = []dns.RR{
		&dns.A{
			Hdr: dns.RR_Header{
				Name:   domain,
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    ttl,
			},
			A: net.ParseIP(ip),
		},
	}
	return msg
}

func TestDNSCacheSetSkipsUncacheable(t *testing.T) {
	noQuestion := createTestMsg("noquestion.example.com.", "192.0.2.1", 60)
	noQuestion.Question = nil

	servfail := createTestMsg("servfail.example.com.", "192.0.2.2", 60)
	servfail.Rcode = dns.RcodeServerFailure

	refused := createTestMsg("refused.example.com.", "192.0.2.3", 60)
	refused.Rcode = dns.RcodeRefused

	nxdomain := new(dns.Msg)
	nxdomain.SetQuestion("nonexistent.example.com.", dns.TypeA)
	nxdomain.Rcode = dns.RcodeNameError

	tests := []struct {
		name    string
		msg     *dns.Msg
		wantHit bool
	}{
		{name: "nil message", msg: nil, wantHit: false},
		{name: "empty question section", msg: noQuestion, wantHit: false},
		{name: "SERVFAIL", msg: servfail, wantHit: false},
		{name: "REFUSED", msg: refused, wantHit: false},
		{name: "NXDOMAIN", msg: nxdomain, wantHit: true},
		{name: "NOERROR", msg: createTestMsg("example.com.", "93.184.216.34", 60), wantHit: true},
	}

	cache := New(slog.Default())
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache.Set(tt.name, tt.msg)
			if _, found := cache.Get(tt.name); found != tt.wantHit {
				t.Errorf("cache hit = %v, want %v", found, tt.wantHit)
			}
		})
	}
}

// TestDNSCacheGetExpiredWithoutQuestion reproduces the panic from 0.0.7: an expired entry
// whose message has no question section must not crash the debug log in Get.
func TestDNSCacheGetExpiredWithoutQuestion(t *testing.T) {
	cache := New(slog.Default())
	cache.cache["broken"] = CacheEntry{Msg: new(dns.Msg), ExpiresAt: time.Now().Add(-time.Second)}

	if _, found := cache.Get("broken"); found {
		t.Error("expected expired entry to be a cache miss")
	}
}

func TestDNSCacheSetStoresCopy(t *testing.T) {
	cache := New(slog.Default())
	msg := createTestMsg("copy.example.com.", "192.0.2.4", 60)
	cache.Set("copy", msg)

	msg.Question = nil
	msg.Answer = nil

	cached, found := cache.Get("copy")
	if !found {
		t.Fatal("expected cache hit")
	}
	if len(cached.Question) != 1 || len(cached.Answer) != 1 {
		t.Errorf("cached message was modified through caller pointer: %d questions, %d answers",
			len(cached.Question), len(cached.Answer))
	}
}

// fakeClock is a manually advanced clock for deterministic TTL tests.
type fakeClock struct{ t time.Time }

func (f *fakeClock) now() time.Time { return f.t }

func (f *fakeClock) advance(d time.Duration) { f.t = f.t.Add(d) }

func newTestCache() (*DNSCache, *fakeClock) {
	clock := &fakeClock{t: time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)}
	cache := New(slog.Default())
	cache.now = clock.now
	cache.lastSweep = clock.t
	return cache, clock
}

func newNXDomainMsg(domain string, soaTTL, soaMinimum uint32) *dns.Msg {
	msg := new(dns.Msg)
	msg.SetQuestion(domain, dns.TypeA)
	msg.Rcode = dns.RcodeNameError
	msg.Ns = []dns.RR{&dns.SOA{
		Hdr:    dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeSOA, Class: dns.ClassINET, Ttl: soaTTL},
		Ns:     "ns1.example.com.",
		Mbox:   "hostmaster.example.com.",
		Minttl: soaMinimum,
	}}
	return msg
}

// TestDNSCacheGetReturnsRemainingTTL makes sure a cache hit carries the remaining TTL instead of the
// TTL from the time the response was stored. Otherwise a downstream cache asking shortly before
// expiry would keep the record for another full TTL.
func TestDNSCacheGetReturnsRemainingTTL(t *testing.T) {
	cache, clock := newTestCache()
	cache.Set("key", createTestMsg("example.com.", "192.0.2.1", 86400))

	tests := []struct {
		advance time.Duration
		wantTTL uint32
	}{
		{advance: 0, wantTTL: 86400},
		{advance: 1500 * time.Millisecond, wantTTL: 86399},
		{advance: 3600 * time.Second, wantTTL: 82799},
		{advance: 82798 * time.Second, wantTTL: 1},
	}
	for _, tt := range tests {
		clock.advance(tt.advance)
		cached, found := cache.Get("key")
		if !found {
			t.Fatalf("expected cache hit at %v", clock.t)
		}
		if got := cached.Answer[0].Header().Ttl; got != tt.wantTTL {
			t.Errorf("TTL at %v = %d, want %d", clock.t, got, tt.wantTTL)
		}
	}

	clock.advance(time.Second)
	if _, found := cache.Get("key"); found {
		t.Error("expected cache miss after the TTL has passed")
	}
}

func TestDNSCacheGetDoesNotModifyStoredEntry(t *testing.T) {
	cache, clock := newTestCache()
	cache.Set("key", createTestMsg("example.com.", "192.0.2.1", 300))

	clock.advance(100 * time.Second)
	first, _ := cache.Get("key")
	first.Answer[0].Header().Ttl = 5

	second, found := cache.Get("key")
	if !found {
		t.Fatal("expected cache hit")
	}
	if got := second.Answer[0].Header().Ttl; got != 200 {
		t.Errorf("TTL = %d, want 200 (stored entry must not be changed by callers or by Get)", got)
	}
}

func TestDNSCacheDecrementsAllSectionsButNotOPT(t *testing.T) {
	cache, clock := newTestCache()
	msg := createTestMsg("www.example.com.", "192.0.2.1", 300)
	msg.Ns = []dns.RR{&dns.NS{
		Hdr: dns.RR_Header{Name: "example.com.", Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: 600},
		Ns:  "ns1.example.com.",
	}}
	msg.Extra = []dns.RR{&dns.A{
		Hdr: dns.RR_Header{Name: "ns1.example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 900},
		A:   net.ParseIP("192.0.2.53"),
	}}
	msg.SetEdns0(4096, true)
	optTTL := msg.IsEdns0().Hdr.Ttl
	cache.Set("key", msg)

	clock.advance(100 * time.Second)
	cached, found := cache.Get("key")
	if !found {
		t.Fatal("expected cache hit")
	}
	if got := cached.Answer[0].Header().Ttl; got != 200 {
		t.Errorf("answer TTL = %d, want 200", got)
	}
	if got := cached.Ns[0].Header().Ttl; got != 500 {
		t.Errorf("authority TTL = %d, want 500", got)
	}
	if got := cached.Extra[0].Header().Ttl; got != 800 {
		t.Errorf("additional TTL = %d, want 800", got)
	}
	opt := cached.IsEdns0()
	if opt == nil {
		t.Fatal("OPT record missing from cached response")
	}
	if opt.Hdr.Ttl != optTTL || !opt.Do() {
		t.Errorf("OPT header changed: ttl field %d, want %d; DO bit %v", opt.Hdr.Ttl, optTTL, opt.Do())
	}
}

func TestDNSCacheExpiresOnLowestTTL(t *testing.T) {
	cache, clock := newTestCache()
	msg := createTestMsg("www.example.com.", "192.0.2.1", 3600)
	msg.Answer = append([]dns.RR{&dns.CNAME{
		Hdr:    dns.RR_Header{Name: "alias.example.com.", Rrtype: dns.TypeCNAME, Class: dns.ClassINET, Ttl: 3600},
		Target: "www.example.com.",
	}}, &dns.A{
		Hdr: dns.RR_Header{Name: "www.example.com.", Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 30},
		A:   net.ParseIP("192.0.2.2"),
	})
	cache.Set("key", msg)

	clock.advance(29 * time.Second)
	if _, found := cache.Get("key"); !found {
		t.Fatal("expected cache hit before the lowest TTL has passed")
	}
	clock.advance(time.Second)
	if _, found := cache.Get("key"); found {
		t.Error("expected cache miss once the lowest TTL in the response has passed")
	}
}

func TestDNSCacheNegativeAnswerUsesSOAMinimum(t *testing.T) {
	tests := []struct {
		name       string
		soaTTL     uint32
		soaMinimum uint32
		wantExpiry time.Duration
	}{
		{name: "SOA minimum lower than SOA TTL", soaTTL: 3600, soaMinimum: 300, wantExpiry: 300 * time.Second},
		{name: "SOA TTL lower than SOA minimum", soaTTL: 120, soaMinimum: 86400, wantExpiry: 120 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache, clock := newTestCache()
			cache.Set("key", newNXDomainMsg("missing.example.com.", tt.soaTTL, tt.soaMinimum))

			clock.advance(tt.wantExpiry - time.Second)
			if _, found := cache.Get("key"); !found {
				t.Fatal("expected cache hit before negative TTL has passed")
			}
			clock.advance(time.Second)
			if _, found := cache.Get("key"); found {
				t.Error("expected cache miss once negative TTL has passed")
			}
		})
	}
}

func TestDNSCacheSkipsZeroTTL(t *testing.T) {
	cache, _ := newTestCache()
	cache.Set("key", createTestMsg("example.com.", "192.0.2.1", 0))
	if _, found := cache.Get("key"); found {
		t.Error("responses with TTL 0 must not be cached")
	}
}

func TestDNSCacheSweepsExpiredEntries(t *testing.T) {
	cache, clock := newTestCache()
	cache.Set("short", createTestMsg("short.example.com.", "192.0.2.1", 10))
	cache.Set("long", createTestMsg("long.example.com.", "192.0.2.2", 3600))

	clock.advance(30 * time.Second)
	cache.Set("trigger", createTestMsg("trigger.example.com.", "192.0.2.3", 3600))
	if got := len(cache.cache); got != 3 {
		t.Fatalf("entries before sweep interval = %d, want 3 (no sweep yet)", got)
	}

	clock.advance(sweepInterval)
	cache.Set("trigger2", createTestMsg("trigger2.example.com.", "192.0.2.4", 3600))
	if _, ok := cache.cache["short"]; ok {
		t.Error("expired entry was not swept")
	}
	if got := len(cache.cache); got != 3 {
		t.Errorf("entries after sweep = %d, want 3", got)
	}
}
