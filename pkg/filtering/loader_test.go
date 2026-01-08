package filtering

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCustomListSourcesLoad(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := writeTempFile(t, tmpDir, "custom.txt", "file.example.com\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("url.example.com\n"))
	}))
	defer server.Close()

	sources := BuildSources(Catalog, map[string]ListConfig{}, []string{filePath, server.URL})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	set, err := LoadSources(context.Background(), sources, tmpDir, logger, 0)
	if err != nil {
		t.Fatalf("LoadSources returned error: %v", err)
	}

	if !set.Matches("file.example.com", false) {
		t.Error("expected file.example.com to be present")
	}
	if !set.Matches("url.example.com", false) {
		t.Error("expected url.example.com to be present")
	}
}

func TestLoadSourcesCacheFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cacheDir := t.TempDir()
	source := Source{ID: "test", Location: server.URL, Enabled: true}
	cached := []byte("cached.example.com\n")
	if err := writeCache(cacheDir, source, cached); err != nil {
		t.Fatalf("write cache: %v", err)
	}

	set, err := LoadSources(context.Background(), []Source{source}, cacheDir, logger, 0)
	if err != nil {
		t.Fatalf("LoadSources returned error: %v", err)
	}
	if !set.Matches("cached.example.com", false) {
		t.Error("expected cached.example.com to be loaded from cache")
	}
}

func TestLoadSourcesCacheMiss(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	cacheDir := t.TempDir()
	source := Source{ID: "test", Location: server.URL, Enabled: true}

	set, err := LoadSources(context.Background(), []Source{source}, cacheDir, logger, 0)
	if err != nil {
		t.Fatalf("LoadSources returned error: %v", err)
	}
	if set.Matches("cached.example.com", false) {
		t.Error("did not expect cached.example.com to be present")
	}
	if len(set.Exact) != 0 || len(set.Wildcards) != 0 {
		t.Error("expected empty set when download and cache both fail")
	}
}

func TestLoadAllowlistParsesComments(t *testing.T) {
	input := strings.Join([]string{
		"# comment",
		"allow.example.com",
		"; another comment",
		"*.safe.example.com",
	}, "\n")
	set, err := loadAllowlist(strings.NewReader(input), "allowlist", nil, 0)
	if err != nil {
		t.Fatalf("loadAllowlist returned error: %v", err)
	}
	if !set.Matches("allow.example.com", false) {
		t.Error("expected allow.example.com to be allowed")
	}
	if !set.Matches("host.safe.example.com", false) {
		t.Error("expected host.safe.example.com to be allowed")
	}
}
