// Package dnscache provides a lightweight TTL-aware cache for DNS responses.
package dnscache

import (
	"log/slog"
	"sync"
	"time"

	"github.com/miekg/dns"
)

// CacheEntry stores a cached DNS message alongside its expiration time.
type CacheEntry struct {
	Msg       *dns.Msg
	ExpiresAt time.Time
}

// DNSCache caches DNS responses keyed by question so repeat lookups are faster.
type DNSCache struct {
	mu    sync.RWMutex
	cache map[string]CacheEntry
	log   *slog.Logger
}

// New constructs a DNSCache that logs cache events.
func New(log *slog.Logger) *DNSCache {
	return &DNSCache{
		cache: make(map[string]CacheEntry),
		log:   log,
	}
}

// Get returns the cached message when it exists and is still valid.
func (c *DNSCache) Get(key string) (*dns.Msg, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	entry, found := c.cache[key]
	if !found {
		return nil, false
	}
	if time.Now().After(entry.ExpiresAt) {
		c.log.Debug("cache expired", "name", entry.Msg.Question[0].Name)
		return nil, false
	}
	return entry.Msg, true
}

// Set stores a DNS response with an expiration derived from the record TTL.
func (c *DNSCache) Set(key string, msg *dns.Msg) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ttl := time.Duration(60) * time.Second
	if len(msg.Answer) > 0 {
		ttl = time.Duration(msg.Answer[0].Header().Ttl) * time.Second
	}
	c.cache[key] = CacheEntry{Msg: msg, ExpiresAt: time.Now().Add(ttl)}
}

// Clear drops the entire cache and logs the action.
func (c *DNSCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[string]CacheEntry)
	c.log.Info("cache cleared")
}
