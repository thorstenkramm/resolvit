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
