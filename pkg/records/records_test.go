package records

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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
a.b.c.example.com A 192.168.1.2
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
	if len(GetAll()) != 4 {
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
			name:     "Double Wildcard record",
			domain:   "Any.Any.example.COM.",
			wantType: A,
			wantIP:   "192.168.1.100",
			wantOK:   true,
		},
		{
			name:     "Triple Wildcard record",
			domain:   "Any.Any.Any.example.COM.",
			wantType: A,
			wantIP:   "192.168.1.100",
			wantOK:   true,
		},
		{
			name:     "Exact match within wildcard record range",
			domain:   "a.b.c.example.com.",
			wantType: A,
			wantIP:   "192.168.1.2",
			wantOK:   true,
		},
		{
			name:   "Non-existent record",
			domain: "notfound.example.org",
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

func TestFollowCNAME(t *testing.T) {
	// The .test names do not match any wildcard left behind by other tests
	Add("follow-a.test", A, "192.0.2.1")
	Add("follow-alias1.test", CNAME, "follow-alias2.test")
	Add("follow-alias2.test", CNAME, "follow-a.test")
	Add("follow-external.test", CNAME, "Target.Example.NET")
	Add("follow-dotted.test", CNAME, "target.example.net.")
	Add("follow-to-txt.test", CNAME, "follow-txt.test")
	Add("follow-txt.test", "txt", "some text")
	Add("follow-self.test", CNAME, "follow-self.test")
	Add("follow-loop-a.test", CNAME, "follow-loop-b.test")
	Add("follow-loop-b.test", CNAME, "follow-loop-a.test")
	Add("follow-into-loop.test", CNAME, "follow-loop-a.test")
	Add("*.follow-wild.test", CNAME, "x.follow-wild.test")
	Add("*.follow-wild-ok.test", CNAME, "x.follow-wild-ok.test")
	Add("x.follow-wild-ok.test", A, "192.0.2.2")

	// follow-max1 starts a chain of exactly MaxCNAMEChain CNAME records, follow-max0 adds one more
	for i := 0; i < MaxCNAMEChain; i++ {
		Add(fmt.Sprintf("follow-max%d.test", i), CNAME, fmt.Sprintf("follow-max%d.test", i+1))
	}
	Add(fmt.Sprintf("follow-max%d.test", MaxCNAMEChain), CNAME, "follow-a.test")

	tests := []struct {
		name       string
		start      string
		wantLinks  int
		wantTarget string
		wantType   string
		wantErr    error
	}{
		{name: "A record without CNAME", start: "follow-a.test", wantType: A},
		{name: "unknown name", start: "follow-missing.test"},
		{name: "two CNAMEs to a local A record", start: "Follow-Alias1.test.", wantLinks: 2, wantTarget: "follow-a.test.", wantType: A},
		{name: "CNAME to an external name", start: "follow-external.test", wantLinks: 1, wantTarget: "target.example.net."},
		{name: "target written with trailing dot", start: "follow-dotted.test", wantLinks: 1, wantTarget: "target.example.net."},
		{name: "CNAME to a record of another type", start: "follow-to-txt.test", wantLinks: 1, wantTarget: "follow-txt.test.", wantType: "txt"},
		{name: "name pointing to itself", start: "follow-self.test", wantErr: ErrCNAMELoop},
		{name: "two names pointing to each other", start: "follow-loop-a.test", wantErr: ErrCNAMELoop},
		{name: "chain leading into a loop", start: "follow-into-loop.test", wantErr: ErrCNAMELoop},
		{name: "wildcard matching its own target", start: "foo.follow-wild.test", wantErr: ErrCNAMELoop},
		{name: "wildcard record itself", start: "*.follow-wild.test", wantErr: ErrCNAMELoop},
		{name: "wildcard with exact record for its target", start: "foo.follow-wild-ok.test", wantLinks: 1,
			wantTarget: "x.follow-wild-ok.test.", wantType: A},
		{name: "chain of exactly the maximum length", start: "follow-max1.test", wantLinks: MaxCNAMEChain,
			wantTarget: "follow-a.test.", wantType: A},
		{name: "chain longer than the maximum", start: "follow-max0.test", wantErr: ErrCNAMEChainTooLong},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			links, end, err := FollowCNAME(tt.start)

			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("got error %v, want %v", err, tt.wantErr)
			}
			if len(links) != tt.wantLinks {
				t.Fatalf("got %d links %v, want %d", len(links), links, tt.wantLinks)
			}
			if tt.wantLinks > 0 {
				if want := normalizeName(tt.start); links[0].Name != want {
					t.Errorf("got first link name %q, want %q", links[0].Name, want)
				}
				if got := links[len(links)-1].Target; got != tt.wantTarget {
					t.Errorf("got last target %q, want %q", got, tt.wantTarget)
				}
			}

			gotType := ""
			if end != nil {
				gotType = end.Typ
			}
			if gotType != tt.wantType {
				t.Errorf("got end record type %q, want %q", gotType, tt.wantType)
			}
		})
	}
}

func TestLoadFromFileWarnsAboutCNAMELoops(t *testing.T) {
	var logBuf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&logBuf, nil))

	recordsFile := filepath.Join(t.TempDir(), "records.txt")
	content := []byte(`
ok.example.com A 192.168.1.1
alias.example.com CNAME ok.example.com
external.example.com CNAME www.example.net
self.example.com CNAME self.example.com
loop-a.example.com CNAME loop-b.example.com
loop-b.example.com CNAME loop-a.example.com
*.wild.example.com CNAME x.wild.example.com
*.fine.example.com CNAME ok.example.com
`)
	if err := os.WriteFile(recordsFile, content, 0600); err != nil {
		t.Fatal(err)
	}

	// A walk that took the read lock again would block here forever
	if err := LoadFromFile(recordsFile, log); err != nil {
		t.Fatal(err)
	}

	if got := len(GetAll()); got != 8 {
		t.Errorf("got %d records, want all 8 to be loaded", got)
	}

	logText := logBuf.String()
	for _, name := range []string{"self.example.com.", "loop-a.example.com.", "loop-b.example.com.", "*.wild.example.com."} {
		if !strings.Contains(logText, "name="+name+" ") {
			t.Errorf("expected a warning for %s, got log:\n%s", name, logText)
		}
	}
	if got := strings.Count(logText, "CNAME record cannot be resolved"); got != 4 {
		t.Errorf("got %d loop warnings, want 4, log:\n%s", got, logText)
	}
	if got := strings.Count(logText, ErrCNAMELoop.Error()); got != 4 {
		t.Errorf("got %d warnings naming the loop error, want 4, log:\n%s", got, logText)
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
