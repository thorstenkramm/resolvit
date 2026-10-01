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
