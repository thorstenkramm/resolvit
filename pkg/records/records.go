package records

import (
	"bufio"
	"log/slog"
	"os"
	"strings"
)

const (
	A     = "a"
	CNAME = "cname"
)

type Record struct {
	Typ     string
	Content string
}

var records = map[string]Record{}

func LoadFromFile(filename string, log *slog.Logger) error {
	file, err := os.Open(filename)
	if err != nil {
		log.Error("Failed to open file", "filename", filename, "error", err)
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) != 3 {
			log.Warn("Skipping invalid line", "line", line)
			continue
		}

		name := fields[0]
		if !strings.HasSuffix(name, ".") {
			name = name + "."
		}

		Add(name, fields[1], fields[2])
		log.Debug("Added record", "name", name, "type", fields[1], "content", fields[2])
	}

	if err := scanner.Err(); err != nil {
		log.Error("Error scanning file", "filename", filename, "error", err)
		return err
	}

	log.Info("Successfully loaded records from file", "filename", filename, "numRecords", len(records))
	return nil
}
func Get(name string) *Record {
	// Try exact match first
	if value, ok := records[name]; ok {
		return &value
	}
	// If no exact match found, try wildcard
	parts := strings.Split(name, ".")
	if len(parts) > 2 {
		// Replace first part with wildcard
		wildcardName := "*." + strings.Join(parts[1:], ".")
		if value, ok := records[wildcardName]; ok {
			return &value
		}
	}
	return nil
}

func GetAll() map[string]Record {
	return records
}

func Add(name string, typ string, content string) {
	records[name] = Record{
		Typ:     strings.ToLower(typ),
		Content: content,
	}
}
