package filtering

import (
	"context"
	"log/slog"
	"sync/atomic"
	"time"
)

// Manager keeps an updated in-memory blocklist set.
type Manager struct {
	sources        []Source
	cacheDir       string
	updateInterval time.Duration
	errorLimit     int
	log            *slog.Logger
	current        atomic.Value
}

// NewManager creates a new Manager instance.
func NewManager(sources []Source, cacheDir string, updateInterval time.Duration, log *slog.Logger, errorLimit int) *Manager {
	if log == nil {
		log = slog.Default()
	}
	m := &Manager{
		sources:        sources,
		cacheDir:       cacheDir,
		updateInterval: updateInterval,
		errorLimit:     errorLimit,
		log:            log,
	}
	m.current.Store(NewDomainSet())
	return m
}

// LoadOnce refreshes the in-memory blocklist set.
func (m *Manager) LoadOnce(ctx context.Context) error {
	set, err := LoadSources(ctx, m.sources, m.cacheDir, m.log, m.errorLimit)
	if err != nil {
		return err
	}
	m.current.Store(set)
	return nil
}

// Start begins a background refresh loop.
func (m *Manager) Start(ctx context.Context) {
	if m.updateInterval <= 0 {
		return
	}
	ticker := time.NewTicker(m.updateInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := m.LoadOnce(ctx); err != nil {
					m.log.Error("failed to refresh blocklists", "error", err)
				}
			}
		}
	}()
}

// Current returns the most recently loaded blocklist set.
func (m *Manager) Current() *DomainSet {
	if value := m.current.Load(); value != nil {
		if set, ok := value.(*DomainSet); ok {
			return set
		}
	}
	return NewDomainSet()
}
