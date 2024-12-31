package records

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromFile(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create temp test directory
	tmpDir := t.TempDir()
	recordsFile := filepath.Join(tmpDir, "records.txt")

	// Test records content
	content := []byte(`
# This is a comment
test1.example.com A 192.168.1.1
test2.exAmple.com CNAME test1.example.com
*.Example.com A 192.168.1.100
test.example.com AAAA 192.168.1.100
test.example.com A 192.168.1.300
`)

	if err := os.WriteFile(recordsFile, content, 0600); err != nil {
		t.Fatal(err)
	}

	if err := LoadFromFile(recordsFile, log); err != nil {
		t.Fatal(err)
	}

	t.Logf("Having %d records", len(GetAll()))
	t.Logf("Records %s", GetAll())
	if len(GetAll()) != 3 {
		t.Fatal("Expected 3 records")
	}

	tests := []struct {
		name     string
		domain   string
		wantType string
		wantIP   string
		wantOK   bool
	}{
		{
			name:     "A record",
			domain:   "test1.example.Com.",
			wantType: A,
			wantIP:   "192.168.1.1",
			wantOK:   true,
		},
		{
			name:     "CNAME record",
			domain:   "Test2.example.Com.",
			wantType: CNAME,
			wantIP:   "test1.example.com",
			wantOK:   true,
		},
		{
			name:     "Wildcard record",
			domain:   "Any.example.COM.",
			wantType: A,
			wantIP:   "192.168.1.100",
			wantOK:   true,
		},
		{
			name:   "Non-existent record",
			domain: "notfound.example.com",
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := Get(tt.domain)
			if !tt.wantOK {
				if record != nil {
					t.Error("expected no record, got one")
				}
				return
			}

			if record == nil {
				t.Fatalf("expected record %s, got nil", tt.domain)
			}

			if record.Typ != tt.wantType {
				t.Errorf("got type %s, want %s", record.Typ, tt.wantType)
			}

			if record.Content != tt.wantIP {
				t.Errorf("got content %s, want %s", record.Content, tt.wantIP)
			}
		})
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
