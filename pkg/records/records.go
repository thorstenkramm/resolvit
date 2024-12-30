package records

import (
	"bufio"
	"log/slog"
	"os"
	"strings"
	"sync"
)

type Record struct {
	Typ     string
	Content string
}

var (
	records = make(map[string]Record)
	mu      sync.RWMutex
)

const (
	CNAME = "cname"
	A     = "a"
)

func Get(name string) *Record {
	mu.RLock()
	defer mu.RUnlock()

	if record, ok := records[name]; ok {
		return &record
	}

	// Try wildcard match
	parts := strings.Split(name, ".")
	if len(parts) > 2 {
		wildcardName := "*." + strings.Join(parts[1:], ".")
		if record, ok := records[wildcardName]; ok {
			return &record
		}
	}

	return nil
}

func GetAll() map[string]Record {
	mu.RLock()
	defer mu.RUnlock()

	return records
}

func Add(name string, typ string, content string) {
	mu.Lock()
	defer mu.Unlock()

	records[name] = Record{
		Typ:     strings.ToLower(typ),
		Content: content,
	}
}

func LoadFromFile(filename string, log *slog.Logger) error {
	mu.Lock()
	defer mu.Unlock()

	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

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

		name := fields[0]
		if !strings.HasSuffix(name, ".") {
			name = name + "."
		}

		records[name] = Record{
			Typ:     strings.ToLower(fields[1]),
			Content: fields[2],
		}
	}

	return scanner.Err()
}
