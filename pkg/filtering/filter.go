package filtering

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"
)

// Filter coordinates blocklist and allowlist checks.
type Filter struct {
	enabled         bool
	blockSubdomains bool
	allowlistPath   string
	sources         []Source
	cacheDir        string
	updateInterval  time.Duration
	errorLimit      int
	log             *slog.Logger
	blockedLogger   *blockedLogger
	blocklist       atomic.Value
	allowlist       atomic.Value
}

// FilterOptions configures a Filter.
type FilterOptions struct {
	Enabled         bool
	BlockSubdomains bool
	AllowlistPath   string
	Sources         []Source
	CacheDir        string
	UpdateInterval  time.Duration
	BlockedLogPath  string
	Log             *slog.Logger
	ErrorLimit      int
}

// NewFilter constructs a Filter instance.
func NewFilter(opts FilterOptions) *Filter {
	log := opts.Log
	if log == nil {
		log = slog.Default()
	}
	filter := &Filter{
		enabled:         opts.Enabled,
		blockSubdomains: opts.BlockSubdomains,
		allowlistPath:   opts.AllowlistPath,
		sources:         opts.Sources,
		cacheDir:        opts.CacheDir,
		updateInterval:  opts.UpdateInterval,
		errorLimit:      opts.ErrorLimit,
		log:             log,
		blockedLogger:   newBlockedLogger(opts.BlockedLogPath, log),
	}
	filter.blocklist.Store(NewDomainSet())
	filter.allowlist.Store(NewDomainSet())
	return filter
}

// LoadOnce refreshes blocklists and allowlists.
func (f *Filter) LoadOnce(ctx context.Context) {
	if !f.enabled {
		return
	}
	blocklist, err := LoadSources(ctx, f.sources, f.cacheDir, f.log, f.errorLimit)
	if err != nil {
		f.log.Error("failed to load blocklists", "error", err)
		blocklist = NewDomainSet()
	}

	allowlist, err := LoadAllowlist(f.allowlistPath, f.log, f.errorLimit)
	if err != nil {
		f.log.Error("failed to load allowlist", "error", err)
		allowlist = NewDomainSet()
	}

	f.blocklist.Store(blocklist)
	f.allowlist.Store(allowlist)
}

// Start begins the background refresh loop.
func (f *Filter) Start(ctx context.Context) {
	if !f.enabled {
		return
	}
	f.LoadOnce(ctx)
	if f.updateInterval <= 0 {
		return
	}

	ticker := time.NewTicker(f.updateInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				f.LoadOnce(ctx)
			}
		}
	}()
}

// ShouldBlock returns true when the name is blocked by configuration.
func (f *Filter) ShouldBlock(name string) bool {
	if f == nil || !f.enabled {
		return false
	}
	allowlist := f.currentAllowlist()
	if allowlist.Matches(name, f.blockSubdomains) {
		return false
	}
	return f.currentBlocklist().Matches(name, f.blockSubdomains)
}

// LogBlocked writes a blocked query entry when configured.
func (f *Filter) LogBlocked(remoteAddr string, name string, qtype uint16) {
	if f == nil || !f.enabled {
		return
	}
	if f.blockedLogger == nil {
		return
	}
	f.blockedLogger.Log(remoteAddr, name, qtype)
}

func (f *Filter) currentBlocklist() *DomainSet {
	if value := f.blocklist.Load(); value != nil {
		if set, ok := value.(*DomainSet); ok {
			return set
		}
	}
	return NewDomainSet()
}

func (f *Filter) currentAllowlist() *DomainSet {
	if value := f.allowlist.Load(); value != nil {
		if set, ok := value.(*DomainSet); ok {
			return set
		}
	}
	return NewDomainSet()
}
