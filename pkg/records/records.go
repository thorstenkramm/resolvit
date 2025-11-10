// Package records stores and serves locally configured DNS records.
package records

import (
	"bufio"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Record represents a single DNS record entry.
type Record struct {
	Typ     string
	Content string
}

var (
	records = make(map[string]Record)
	mu      sync.RWMutex
)

const (
	// CNAME is the canonical name record identifier used in record files.
	CNAME = "cname"
	// A is the IPv4 host record identifier used in record files.
	A = "a"
)

// Get looks up a record by name, supporting wildcard matches.
func Get(name string) *Record {
	mu.RLock()
	defer mu.RUnlock()

	name = strings.ToLower(name)
	if record, ok := records[name]; ok {
		return &record
	}

	// Try wildcard match
	parts := strings.Split(name, ".")
	for i := 1; i < len(parts)-1; i++ {
		wildcardName := "*." + strings.Join(parts[i:], ".")
		if record, ok := records[wildcardName]; ok {
			return &record
		}
	}

	return nil
}

// GetAll returns the in-memory record map for inspection or testing.
func GetAll() map[string]Record {
	mu.RLock()
	defer mu.RUnlock()

	return records
}

// Add inserts or updates a record in the in-memory store.
func Add(name string, typ string, content string) {
	mu.Lock()
	defer mu.Unlock()

	records[name] = Record{
		Typ:     strings.ToLower(typ),
		Content: content,
	}
}

// LoadFromFile parses the given file and populates the record store.
func LoadFromFile(filename string, log *slog.Logger) error {
	mu.Lock()
	defer mu.Unlock()

	resolvedPath, err := sanitizeRecordsPath(filename)
	if err != nil {
		return err
	}

	file, err := os.Open(resolvedPath) // #nosec G304 -- path validated via sanitizeRecordsPath
	if err != nil {
		return err
	}
	defer func() {
		if err := file.Close(); err != nil {
			if log != nil {
				log.Error("failed to close records file", "from_file", filename, "error", err)
			} else {
				slog.Error("failed to close records file", "from_file", filename, "error", err)
			}
		}
	}()

	// Clear existing records
	records = make(map[string]Record)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) != 3 {
			log.Warn("Invalid record format", "line", line)
			continue
		}

		name := strings.ToLower(fields[0])
		if !strings.HasSuffix(name, ".") {
			name = name + "."
		}

		typ := strings.ToLower(fields[1])
		if typ != CNAME && typ != A {
			log.Warn("Invalid record type", "type", typ, "line", line)
			continue
		}

		content := strings.ToLower(fields[2])
		if typ == A {
			if ok := net.ParseIP(content); ok == nil {
				log.Warn("Invalid ipv4 address for record content", "content", content, "line", line)
				continue
			}
		}

		records[name] = Record{
			Typ:     typ,
			Content: content,
		}
	}

	log.Info("Loaded records", "from_file", resolvedPath, "num_records", len(records))

	return scanner.Err()
}

func sanitizeRecordsPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("records path is empty")
	}

	clean := filepath.Clean(path)
	if clean == "." || clean == string(os.PathSeparator) {
		return "", fmt.Errorf("records path %q resolves to a directory", path)
	}

	if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("records path %q escapes the working directory", path)
	}

	return clean, nil
}
