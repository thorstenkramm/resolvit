package filtering

import (
	"fmt"
	"io"
	"log/slog"
	"os"
)

// LoadAllowlist reads the allowlist file and returns a DomainSet.
func LoadAllowlist(path string, log *slog.Logger, errorLimit int) (*DomainSet, error) {
	if path == "" {
		return NewDomainSet(), nil
	}

	file, err := os.Open(path) // #nosec G304 -- path is provided via config.
	if err != nil {
		return nil, fmt.Errorf("open allowlist: %w", err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			if log == nil {
				slog.Default().Warn("failed to close allowlist file", "error", err)
			} else {
				log.Warn("failed to close allowlist file", "error", err)
			}
		}
	}()

	return loadAllowlist(file, "allowlist", log, errorLimit)
}

func loadAllowlist(r io.Reader, listID string, log *slog.Logger, errorLimit int) (*DomainSet, error) {
	set, err := parseList(r, parseOptions{
		ListID:     listID,
		Logger:     log,
		ErrorLimit: errorLimit,
	})
	if err != nil {
		return nil, err
	}
	return set, nil
}
