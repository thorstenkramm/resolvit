package filtering

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultHTTPTimeout = 20 * time.Second
)

// EnsureCacheDir creates the cache directory if missing. Returns an empty string on failure.
func EnsureCacheDir(cacheDir string, log *slog.Logger) string {
	if cacheDir == "" {
		return ""
	}
	if err := os.MkdirAll(cacheDir, 0o750); err != nil {
		if log != nil {
			log.Error("failed to create cache dir, caching disabled", "dir", cacheDir, "error", err)
		}
		return ""
	}
	return cacheDir
}

// LoadSources loads enabled sources, merging them into a single DomainSet.
func LoadSources(ctx context.Context, sources []Source, cacheDir string, log *slog.Logger, errorLimit int) (*DomainSet, error) {
	if log == nil {
		log = slog.Default()
	}

	merged := NewDomainSet()
	cacheDir = EnsureCacheDir(cacheDir, log)

	for _, source := range sources {
		if !source.Enabled {
			continue
		}
		set, err := loadSource(ctx, source, cacheDir, log, errorLimit)
		if err != nil {
			log.Error("failed to load blocklist", "list", source.ID, "error", err)
			continue
		}
		merged.Merge(set)
	}

	return merged, nil
}

func loadSource(ctx context.Context, source Source, cacheDir string, log *slog.Logger, errorLimit int) (*DomainSet, error) {
	data, fromCache, err := readSource(ctx, source, cacheDir, log)
	if err != nil {
		return nil, err
	}

	set, err := parseList(bytes.NewReader(data), parseOptions{
		ListID:     source.ID,
		Logger:     log,
		ErrorLimit: errorLimit,
	})
	if err != nil {
		return nil, err
	}

	if !fromCache && cacheDir != "" && isURL(source.Location) {
		if err := writeCache(cacheDir, source, data); err != nil {
			log.Warn("failed to write cache", "list", source.ID, "error", err)
		}
	}

	return set, nil
}

func readSource(ctx context.Context, source Source, cacheDir string, log *slog.Logger) ([]byte, bool, error) {
	if isURL(source.Location) {
		data, err := download(ctx, source)
		if err == nil {
			return data, false, nil
		}
		if cacheDir == "" {
			return nil, false, err
		}
		cached, cacheErr := readCache(cacheDir, source)
		if cacheErr != nil {
			return nil, false, fmt.Errorf("download failed: %w; cache error: %s", err, cacheErr.Error())
		}
		log.Warn("download failed, using cached list", "list", source.ID, "error", err)
		return cached, true, nil
	}

	data, err := os.ReadFile(source.Location)
	if err != nil {
		return nil, false, fmt.Errorf("read file: %w", err)
	}
	return data, false, nil
}

func download(ctx context.Context, source Source) ([]byte, error) {
	client := &http.Client{Timeout: defaultHTTPTimeout}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source.Location, nil)
	if err != nil {
		return nil, err
	}
	applyAuth(req, source.Auth)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Default().Warn("failed to close blocklist response body", "error", err)
		}
	}()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}

	return io.ReadAll(resp.Body)
}

func applyAuth(req *http.Request, auth AuthConfig) {
	if auth.Username != "" || auth.Password != "" {
		req.SetBasicAuth(auth.Username, auth.Password)
	}
	if auth.Token != "" {
		header := auth.Header
		if header == "" {
			header = "Authorization"
		}
		scheme := auth.Scheme
		if scheme == "" {
			scheme = "Bearer"
		}
		req.Header.Set(header, strings.TrimSpace(scheme+" "+auth.Token))
	}
}

func writeCache(cacheDir string, source Source, data []byte) error {
	path := filepath.Join(cacheDir, cacheFileName(source))
	return os.WriteFile(path, data, 0o600)
}

func readCache(cacheDir string, source Source) ([]byte, error) {
	path := filepath.Join(cacheDir, cacheFileName(source))
	// #nosec G304 -- cache path is derived from configured cache directory.
	return os.ReadFile(path)
}

func cacheFileName(source Source) string {
	id := sanitizeID(source.ID)
	if id == "" {
		hash := sha256.Sum256([]byte(source.Location))
		id = "custom-" + hex.EncodeToString(hash[:8])
	}
	return id + ".txt"
}

func sanitizeID(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}
	builder := strings.Builder{}
	for _, r := range raw {
		switch {
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		default:
			builder.WriteRune('_')
		}
	}
	return builder.String()
}

func isURL(location string) bool {
	return strings.HasPrefix(location, "http://") || strings.HasPrefix(location, "https://")
}
