// Package records stores and serves locally configured DNS records.
package records

import (
	"bufio"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// MaxCNAMEChain is the highest number of local CNAME records followed for one question.
const MaxCNAMEChain = 16

var (
	// ErrCNAMELoop is returned when a local CNAME chain visits a name a second time.
	ErrCNAMELoop = errors.New("cname loop in local records")
	// ErrCNAMEChainTooLong is returned when a local CNAME chain exceeds MaxCNAMEChain.
	ErrCNAMEChainTooLong = errors.New("local cname chain too long")
)

// Record represents a single DNS record entry.
type Record struct {
	Typ     string
	Content string
}

// Link is one CNAME record of a local chain. Name and Target are lower case and end with a dot.
type Link struct {
	Name   string
	Target string
}

var (
	// The in-memory record store is safe for concurrent use via package functions.
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

	return getLocked(normalizeName(name))
}

// getLocked looks up a record by its normalised name, supporting wildcard matches.
// The caller must hold the lock.
func getLocked(name string) *Record {
	if name == "" {
		return nil
	}
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

// FollowCNAME follows the local CNAME records starting at name, supporting wildcard matches.
// It returns the CNAME records in the order they were followed and the record that ended the chain.
// That record is the first one that is not a CNAME. It is nil when the chain ends at a name without
// a local record, which is the target of the last link or, without any link, name itself.
// A chain that visits a name twice yields ErrCNAMELoop, a chain of more than MaxCNAMEChain
// CNAME records yields ErrCNAMEChainTooLong.
// FollowCNAME is safe for concurrent use. All steps see the same set of records.
func FollowCNAME(name string) ([]Link, *Record, error) {
	mu.RLock()
	defer mu.RUnlock()

	return followLocked(normalizeName(name))
}

// followLocked implements FollowCNAME for a normalised name.
// The caller must hold the lock.
func followLocked(name string) ([]Link, *Record, error) {
	var links []Link
	visited := make(map[string]struct{})

	for {
		record := getLocked(name)
		if record == nil || record.Typ != CNAME {
			return links, record, nil
		}
		if len(links) == MaxCNAMEChain {
			return nil, nil, fmt.Errorf("%w: more than %d records, stopped at %s", ErrCNAMEChainTooLong, MaxCNAMEChain, name)
		}

		// Names are remembered, not records: two names may match the same wildcard without forming a loop.
		visited[name] = struct{}{}
		target := normalizeName(record.Content)
		links = append(links, Link{Name: name, Target: target})
		if _, seen := visited[target]; seen {
			return nil, nil, fmt.Errorf("%w: %s is visited twice", ErrCNAMELoop, target)
		}
		name = target
	}
}

// warnUnresolvableCNAMEsLocked logs a warning for every CNAME record whose chain loops or is too long.
// Wildcard records are walked from their own name: the exact lookup finds the wildcard record itself.
// The caller must hold the lock.
func warnUnresolvableCNAMEsLocked(log *slog.Logger) {
	names := make([]string, 0, len(records))
	for name, record := range records {
		if record.Typ == CNAME {
			names = append(names, name)
		}
	}
	slices.Sort(names)

	for _, name := range names {
		if _, _, err := followLocked(name); err != nil {
			log.Warn("CNAME record cannot be resolved", "name", name, "error", err)
		}
	}
}

// GetAll returns a copy of the in-memory record map for inspection or testing.
func GetAll() map[string]Record {
	mu.RLock()
	defer mu.RUnlock()

	copyMap := make(map[string]Record, len(records))
	for name, record := range records {
		copyMap[name] = record
	}
	return copyMap
}

// Add inserts or updates a record in the in-memory store.
func Add(name string, typ string, content string) {
	mu.Lock()
	defer mu.Unlock()

	name = normalizeName(name)
	if name == "" {
		return
	}

	records[name] = Record{
		Typ:     strings.ToLower(typ),
		Content: strings.ToLower(content),
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

		name := normalizeName(fields[0])
		if name == "" {
			log.Warn("Invalid record name", "line", line)
			continue
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

	warnUnresolvableCNAMEsLocked(log)

	log.Info("Loaded records", "from_file", resolvedPath, "num_records", len(records))

	return scanner.Err()
}

// sanitizeRecordsPath rejects empty, root, or parent paths to avoid traversal.
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

func normalizeName(name string) string {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ""
	}
	lower := strings.ToLower(trimmed)
	if strings.HasSuffix(lower, ".") {
		return lower
	}
	return lower + "."
}
