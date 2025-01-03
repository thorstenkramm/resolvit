package main

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
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
	viper.Set("log-level", "error")
	viper.Set("log-file", "stdout")

	// Start server
	go main()
	time.Sleep(1 * time.Second)

	// Test initial records
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

	//runTests(t, c, tests)
	// Create both UDP and TCP clients
	udpClient := new(dns.Client)
	tcpClient := &dns.Client{Net: "tcp"}

	// Run tests with both UDP and TCP
	clients := map[string]*dns.Client{
		"UDP": udpClient,
		"TCP": tcpClient,
	}

	for protocol, c := range clients {
		t.Run(protocol, func(t *testing.T) {
			// Run existing tests with current client
			runTests(t, c, tests)
		})
	}

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

	// Run reload tests with both protocols
	for protocol, c := range clients {
		t.Run("Reload_"+protocol, func(t *testing.T) {
			runTests(t, c, reloadTests)
		})
	}
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

func TestConcurrentRequests(t *testing.T) {
	// Setup same test environment as in TestDNSServer
	tmpDir, err := os.MkdirTemp("", "dns-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	recordsFile := filepath.Join(tmpDir, "records.txt")
	records := []byte(`
my.example.com A 127.0.0.99
test1.example.com A 192.168.1.1
test2.example.com A 192.168.1.2
test3.example.com A 192.168.1.3
`)

	if err := os.WriteFile(recordsFile, records, 0600); err != nil {
		t.Fatal(err)
	}

	// Set test configuration
	viper.Set("upstream", []string{"1.1.1.1:53"})
	viper.Set("listen", "127.0.0.1:5301") // Different port to avoid conflicts
	viper.Set("resolve-from", recordsFile)
	viper.Set("log-level", "error")
	viper.Set("log-file", "stdout")

	// Start server
	go main()
	time.Sleep(1 * time.Second)

	// Test concurrent requests
	concurrentTests := []struct {
		domain      string
		queryType   uint16
		wantType    uint16
		wantContent string
	}{
		{"my.example.com.", dns.TypeA, dns.TypeA, "127.0.0.99"},
		{"test1.example.com.", dns.TypeA, dns.TypeA, "192.168.1.1"},
		{"test2.example.com.", dns.TypeA, dns.TypeA, "192.168.1.2"},
		{"test3.example.com.", dns.TypeA, dns.TypeA, "192.168.1.3"},
		{"google.com.", dns.TypeA, dns.TypeA, ""}, // Will be forwarded
	}

	workers := 100
	requestsPerWorker := 200
	var wg sync.WaitGroup
	errorsChan := make(chan error, workers*requestsPerWorker)

	// Create DNS client pool to avoid sharing clients between goroutines
	clientPool := sync.Pool{
		New: func() interface{} {
			return new(dns.Client)
		},
	}

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()

			// Get client from pool
			c := clientPool.Get().(*dns.Client)
			defer clientPool.Put(c)

			for i := 0; i < requestsPerWorker; i++ {
				randNum, err := rand.Int(rand.Reader, big.NewInt(int64(len(concurrentTests))))
				if err != nil {
					errorsChan <- fmt.Errorf("failed to generate random number: %w", err)
					continue
				}
				test := concurrentTests[randNum.Int64()]

				m := new(dns.Msg)
				m.SetQuestion(test.domain, test.queryType)

				r, _, err := c.Exchange(m, "127.0.0.1:5301")
				if err != nil {
					errorsChan <- fmt.Errorf("query failed for %s: %w", test.domain, err)
					continue
				}

				if r.Rcode != dns.RcodeSuccess {
					errorsChan <- fmt.Errorf("query failed for %s with rcode %d", test.domain, r.Rcode)
					continue
				}

				if len(r.Answer) == 0 && test.wantContent != "" {
					errorsChan <- fmt.Errorf("no answer section in response for %s", test.domain)
				}
			}
		}()
	}

	wg.Wait()
	close(errorsChan)

	var errCount int
	for err := range errorsChan {
		t.Errorf("concurrent test error: %v", err)
		errCount++
	}

	if errCount > 0 {
		t.Fatalf("got %d errors during concurrent testing", errCount)
	}
}
