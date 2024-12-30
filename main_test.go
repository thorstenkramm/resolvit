package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/miekg/dns"
	"github.com/spf13/viper"
)

func TestDNSServer(t *testing.T) {
	// Create temp dir and copy records file
	tmpDir, err := os.MkdirTemp("", "dns-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	recordsFile := filepath.Join(tmpDir, "records.txt")
	initialRecords := []byte(`my.example.com A 127.0.0.99
cname.example.com CNAME my.example.com
cname2.example.com CNAME cname.example.com
google.example.com CNAME google.com
web.example.com A 192.168.1.1`)

	if err := os.WriteFile(recordsFile, initialRecords, 0600); err != nil {
		t.Fatal(err)
	}

	// Set test configuration
	viper.Set("upstream", []string{"1.1.1.1:53"})
	viper.Set("listen", "127.0.0.1:5300")
	viper.Set("resolve-from", recordsFile)
	viper.Set("log-level", "debug")
	viper.Set("log-file", "stdout")

	// Start server
	go main()
	time.Sleep(1 * time.Second)

	// Test initial records
	c := new(dns.Client)
	tests := []struct {
		name        string
		domain      string
		queryType   uint16
		wantType    uint16
		wantContent string
		wantIP      string
	}{
		{
			name:        "Forwarded A record",
			domain:      "heise.de.",
			queryType:   dns.TypeA,
			wantType:    dns.TypeA,
			wantContent: "193.99.144.80",
		},
		{
			name:        "Initial A record",
			domain:      "my.example.com.",
			queryType:   dns.TypeA,
			wantType:    dns.TypeA,
			wantContent: "127.0.0.99",
		},
		{
			name:        "Initial CNAME record",
			domain:      "cname.example.com.",
			queryType:   dns.TypeA,
			wantType:    dns.TypeCNAME,
			wantContent: "my.example.com.",
			wantIP:      "127.0.0.99",
		},
	}

	runTests(t, c, tests)

	// Update records file with new content
	newRecords := []byte(`*.example.com A 192.168.1.100
new.example.com CNAME test.example.com`)

	if err := os.WriteFile(recordsFile, newRecords, 0600); err != nil {
		t.Fatal(err)
	}

	// Send SIGHUP to reload records
	proc, err := os.FindProcess(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	err = proc.Signal(syscall.SIGHUP)
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(1 * time.Second)

	// Test reloaded records
	reloadTests := []struct {
		name        string
		domain      string
		queryType   uint16
		wantType    uint16
		wantContent string
		wantIP      string
	}{
		{
			name:        "New A record",
			domain:      "test.example.com.",
			queryType:   dns.TypeA,
			wantType:    dns.TypeA,
			wantContent: "192.168.1.100",
		},
		{
			name:        "New CNAME record",
			domain:      "new.example.com.",
			queryType:   dns.TypeA,
			wantType:    dns.TypeCNAME,
			wantContent: "test.example.com.",
			wantIP:      "192.168.1.100",
		},
	}

	runTests(t, c, reloadTests)
}

func runTests(t *testing.T, c *dns.Client, tests []struct {
	name        string
	domain      string
	queryType   uint16
	wantType    uint16
	wantContent string
	wantIP      string
}) {
	t.Helper()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := new(dns.Msg)
			m.SetQuestion(tt.domain, tt.queryType)

			r, _, err := c.Exchange(m, "127.0.0.1:5300")
			if err != nil {
				t.Fatalf("Query failed: %v", err)
			}

			if len(r.Answer) == 0 {
				t.Fatal("No answer section in response")
			}

			answer := r.Answer[0]
			if answer.Header().Rrtype != tt.wantType {
				t.Errorf("Got record type %d, want %d", answer.Header().Rrtype, tt.wantType)
			}

			switch tt.wantType {
			case dns.TypeA:
				aRecord := answer.(*dns.A)
				if aRecord.A.String() != tt.wantContent {
					t.Errorf("Got IP %s, want %s", aRecord.A.String(), tt.wantContent)
				}
			case dns.TypeCNAME:
				cnameRecord := answer.(*dns.CNAME)
				if cnameRecord.Target != tt.wantContent {
					t.Errorf("Got target %s, want %s", cnameRecord.Target, tt.wantContent)
				}
				// Validate resolved IP for CNAME
				if tt.wantIP != "" && len(r.Answer) > 1 {
					aRecord := r.Answer[len(r.Answer)-1].(*dns.A)
					if aRecord.A.String() != tt.wantIP {
						t.Errorf("Got resolved IP %s, want %s", aRecord.A.String(), tt.wantIP)
					}
				}
			}
		})
	}
}
