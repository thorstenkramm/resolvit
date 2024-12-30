package records

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFromFile(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create temp file with test records
	content := []string{
		"# Test DNS records",
		"www.google.de A 127.0.0.99",
		"www.google.com    CNAME     cname.sys25.net",
		"www.google.fr	CNAME	www.google.com",
		"www.pupes.de      A    10.10.10.1",
		"www.gaga.de         CNAME       eselsel.de",
	}

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test_records.txt")
	if err := os.WriteFile(tmpFile, []byte(strings.Join(content, "\n")), 0600); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	err := LoadFromFile(tmpFile, log)
	if err != nil {
		t.Fatalf("Failed to load records: %v", err)
	}

	testCases := []struct {
		name          string
		expectedType  string
		expectedValue string
	}{
		{"www.google.de.", "a", "127.0.0.99"},
		{"www.google.com.", "cname", "cname.sys25.net"},
		{"www.google.fr.", "cname", "www.google.com"},
		{"www.pupes.de.", "a", "10.10.10.1"},
		{"www.gaga.de.", "cname", "eselsel.de"},
	}

	for _, tc := range testCases {
		record := Get(tc.name)
		if record == nil {
			t.Errorf("Record not found for %s", tc.name)
			continue
		}
		if record.Typ != tc.expectedType {
			t.Errorf("Wrong type for %s: got %s, want %s", tc.name, record.Typ, tc.expectedType)
		}
		if record.Content != tc.expectedValue {
			t.Errorf("Wrong content for %s: got %s, want %s", tc.name, record.Content, tc.expectedValue)
		}
	}
}

func TestAdd(t *testing.T) {
	tests := []struct {
		name        string
		recordName  string
		recordType  string
		content     string
		wantType    string
		wantContent string
	}{
		{
			name:        "Add A record",
			recordName:  "test.example.com.",
			recordType:  "a",
			content:     "192.168.1.10",
			wantType:    "a",
			wantContent: "192.168.1.10",
		},
		{
			name:        "Add CNAME record",
			recordName:  "alias.example.com.",
			recordType:  "cname",
			content:     "test.example.com",
			wantType:    "cname",
			wantContent: "test.example.com",
		},
		{
			name:        "Add record without trailing dot",
			recordName:  "test2.example.com",
			recordType:  "a",
			content:     "192.168.1.11",
			wantType:    "a",
			wantContent: "192.168.1.11",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			Add(tt.recordName, tt.recordType, tt.content)

			record := Get(tt.recordName)
			if record == nil {
				t.Fatalf("Record not found for %s", tt.recordName)
			}

			if record.Typ != tt.wantType {
				t.Errorf("Wrong type for %s: got %s, want %s", tt.recordName, record.Typ, tt.wantType)
			}

			if record.Content != tt.wantContent {
				t.Errorf("Wrong content for %s: got %s, want %s", tt.recordName, record.Content, tt.wantContent)
			}
		})
	}
}
