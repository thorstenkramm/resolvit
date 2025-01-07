package dnscache

import (
	"log/slog"
	"sync"
	"time"

	"github.com/miekg/dns"
)

type CacheEntry struct {
	Msg       *dns.Msg
	ExpiresAt time.Time
}

type DNSCache struct {
	mu    sync.RWMutex
	cache map[string]CacheEntry
	log   *slog.Logger
}

func New(log *slog.Logger) *DNSCache {
	return &DNSCache{
		cache: make(map[string]CacheEntry),
		log:   log,
	}
}

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

func (c *DNSCache) Set(key string, msg *dns.Msg) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ttl := time.Duration(60) * time.Second
	if len(msg.Answer) > 0 {
		ttl = time.Duration(msg.Answer[0].Header().Ttl) * time.Second
	}
	c.cache[key] = CacheEntry{Msg: msg, ExpiresAt: time.Now().Add(ttl)}
}

func (c *DNSCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache = make(map[string]CacheEntry)
	c.log.Info("cache cleared")
}
