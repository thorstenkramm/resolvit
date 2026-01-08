package filtering

import (
	"fmt"
	"strings"
)

// BuildSources converts list configuration into loadable sources.
func BuildSources(catalog map[string]ListDefinition, configs map[string]ListConfig, custom []string) []Source {
	sources := make([]Source, 0)

	for id, cfg := range configs {
		if !cfg.Enabled {
			continue
		}
		location := cfg.URL
		if location == "" {
			if def, ok := catalog[id]; ok {
				location = def.URL
			}
		}
		if location == "" {
			continue
		}
		sources = append(sources, Source{
			ID:       id,
			Location: location,
			Enabled:  true,
			Auth: AuthConfig{
				Username: cfg.Username,
				Password: cfg.Password,
				Token:    cfg.Token,
				Header:   cfg.Header,
				Scheme:   cfg.Scheme,
			},
		})
	}

	for i, entry := range custom {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			continue
		}
		sources = append(sources, Source{
			ID:       fmt.Sprintf("custom_%d", i+1),
			Location: trimmed,
			Enabled:  true,
		})
	}

	return sources
}
