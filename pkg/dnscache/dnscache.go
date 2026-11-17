// Package dnscache provides a lightweight TTL-aware cache for DNS responses.
package dnscache

import (
	"log/slog"
	"sync"
	"time"

	"github.com/miekg/dns"
)

const (
	// defaultTTL is used for cacheable responses that carry no records at all.
	defaultTTL = 60 * time.Second
	// sweepInterval is the minimum time between two sweeps that drop expired entries.
	sweepInterval = time.Minute
)

// CacheEntry stores a cached DNS message alongside the time it was stored and its expiration time.
type CacheEntry struct {
	Msg       *dns.Msg
	StoredAt  time.Time
	ExpiresAt time.Time
}

// DNSCache caches DNS responses keyed by question so repeat lookups are faster.
// DNSCache is safe for concurrent use.
type DNSCache struct {
	mu        sync.RWMutex
	cache     map[string]CacheEntry
	log       *slog.Logger
	now       func() time.Time
	lastSweep time.Time
}

// New constructs a DNSCache that logs cache events.
func New(log *slog.Logger) *DNSCache {
	return &DNSCache{
		cache:     make(map[string]CacheEntry),
		log:       log,
		now:       time.Now,
		lastSweep: time.Now(),
	}
}

// Get returns a copy of the cached message when it exists and is still valid.
// The TTLs of all records in the copy are reduced by the time the entry has spent in the cache,
// so downstream caches never keep a record longer than the upstream TTL allows.
func (c *DNSCache) Get(key string) (*dns.Msg, bool) {
	c.mu.RLock()
	entry, found := c.cache[key]
	c.mu.RUnlock()
	if !found {
		return nil, false
	}
	now := c.now()
	if !now.Before(entry.ExpiresAt) {
		c.log.Debug("cache expired", "key", key)
		return nil, false
	}
	msg := entry.Msg.Copy()
	decrementTTLs(msg, now.Sub(entry.StoredAt))
	return msg, true
}

// Set stores a copy of a DNS response with an expiration derived from the lowest TTL in the response.
// Responses without a question section and responses with an rcode other than
// NOERROR or NXDOMAIN (e.g. SERVFAIL, REFUSED, FORMERR from a struggling upstream) are not cached.
// Responses whose lowest TTL is zero are not cached either.
func (c *DNSCache) Set(key string, msg *dns.Msg) {
	if !cacheable(msg) {
		c.log.Debug("response not cacheable", "key", key)
		return
	}
	ttl := responseTTL(msg)
	if ttl <= 0 {
		c.log.Debug("response has zero TTL, not caching", "key", key)
		return
	}
	now := c.now()
	entry := CacheEntry{Msg: msg.Copy(), StoredAt: now, ExpiresAt: now.Add(ttl)}
	// Negative answers must advertise the effective SOA TTL, so downstream caches
	// count down from the same lifetime we use locally (RFC 2308).
	for _, rr := range entry.Msg.Ns {
		if soa, ok := rr.(*dns.SOA); ok {
			soa.Hdr.Ttl = min(soa.Hdr.Ttl, soa.Minttl)
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[key] = entry
	c.sweepExpiredLocked(now)
}

// sweepExpiredLocked drops expired entries at most once per sweepInterval.
// Without it, entries for names that are never queried again would stay in memory forever.
// The caller must hold the write lock.
func (c *DNSCache) sweepExpiredLocked(now time.Time) {
	if now.Sub(c.lastSweep) < sweepInterval {
		return
	}
	c.lastSweep = now
	removed := 0
	for key, entry := range c.cache {
		if !now.Before(entry.ExpiresAt) {
			delete(c.cache, key)
			removed++
		}
	}
	if removed > 0 {
		c.log.Debug("swept expired cache entries", "removed", removed, "remaining", len(c.cache))
	}
}

func cacheable(msg *dns.Msg) bool {
	if msg == nil || len(msg.Question) == 0 {
		return false
	}
	return msg.Rcode == dns.RcodeSuccess || msg.Rcode == dns.RcodeNameError
}

// responseTTL returns the lowest TTL of all records in the response, ignoring the EDNS OPT pseudo-record.
// For SOA records in the authority section (negative answers, RFC 2308) the SOA MINIMUM field is
// taken into account as well.
// Responses without any records fall back to defaultTTL.
func responseTTL(msg *dns.Msg) time.Duration {
	lowest := uint32(0)
	found := false
	for i, section := range [][]dns.RR{msg.Answer, msg.Ns, msg.Extra} {
		for _, rr := range section {
			if rr.Header().Rrtype == dns.TypeOPT {
				continue
			}
			ttl := rr.Header().Ttl
			if soa, ok := rr.(*dns.SOA); ok && i == 1 && soa.Minttl < ttl {
				ttl = soa.Minttl
			}
			if !found || ttl < lowest {
				lowest = ttl
				found = true
			}
		}
	}
	if !found {
		return defaultTTL
	}
	return time.Duration(lowest) * time.Second
}

// decrementTTLs reduces the TTL of every record by the given age, never going below one second.
// The EDNS OPT pseudo-record is left untouched because its TTL field carries flags, not a TTL.
func decrementTTLs(msg *dns.Msg, age time.Duration) {
	elapsed := int64(age / time.Second)
	if elapsed < 0 {
		elapsed = 0
	}
	for _, section := range [][]dns.RR{msg.Answer, msg.Ns, msg.Extra} {
		for _, rr := range section {
			hdr := rr.Header()
			if hdr.Rrtype == dns.TypeOPT {
				continue
			}
			remaining := int64(hdr.Ttl) - elapsed
			hdr.Ttl = uint32(max(remaining, 1)) // #nosec G115 -- remaining is between 1 and the original uint32 TTL
		}
	}
}

// Clear drops the entire cache and logs the action.
func (c *DNSCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[string]CacheEntry)
	c.log.Info("cache cleared")
}
