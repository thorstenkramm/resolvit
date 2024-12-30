package resolve

import (
	"testing"
)

func TestHost(t *testing.T) {
	testCases := []struct {
		name    string
		host    string
		wantIPs bool
		dnsAddr string
	}{
		{
			name:    "Valid host google-public-dns",
			host:    "dns.google",
			wantIPs: true,
			dnsAddr: "8.8.8.8:53",
		},
		{
			name:    "Valid host cloudflare",
			host:    "one.one.one.one",
			wantIPs: true,
			dnsAddr: "1.1.1.1:53",
		},
		{
			name:    "Invalid host",
			host:    "nonexistent.example.invalid",
			wantIPs: false,
			dnsAddr: "8.8.8.8:53",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ips, err := Host(tc.host, tc.dnsAddr)
			if err != nil && tc.wantIPs {
				t.Errorf("Host(%q, %q) failed: %v", tc.host, tc.dnsAddr, err)
			}
			if tc.wantIPs && len(ips) == 0 {
				t.Errorf("Expected IPs for %s but got none", tc.host)
			}
			if !tc.wantIPs && len(ips) > 0 {
				t.Errorf("Expected no IPs for %s but got %v", tc.host, ips)
			}
		})
	}
}
