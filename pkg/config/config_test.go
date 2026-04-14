package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateLogLevel(t *testing.T) {
	validLevels := []string{"debug", "info", "warn", "error", "DEBUG", "INFO", "WARN", "ERROR"}
	for _, level := range validLevels {
		if err := ValidateLogLevel(level); err != nil {
			t.Errorf("ValidateLogLevel(%s) returned error: %v", level, err)
		}
	}

	invalidLevels := []string{"", "trace", "fatal", "invalid", "debugging"}
	for _, level := range invalidLevels {
		if err := ValidateLogLevel(level); err == nil {
			t.Errorf("ValidateLogLevel(%s) should return error", level)
		}
	}
}

func TestValidateAddress(t *testing.T) {
	validAddresses := []string{
		"127.0.0.1:53",
		"0.0.0.0:5300",
		"8.8.8.8:53",
		"192.168.1.1:5353",
	}
	for _, addr := range validAddresses {
		if err := ValidateAddress(addr); err != nil {
			t.Errorf("ValidateAddress(%s) returned error: %v", addr, err)
		}
	}

	invalidAddresses := []string{
		"localhost:53",       // not IP
		"127.0.0.1",          // no port
		"256.256.256.256:53", // invalid IP
		"8.8.8.8:999999",     // invalid port
		"8.8.8.8:-1",         // negative port
		":53",                // missing IP
		"127.0.0.1:",         // missing port
	}
	for _, addr := range invalidAddresses {
		if err := ValidateAddress(addr); err == nil {
			t.Errorf("ValidateAddress(%s) should return error", addr)
		}
	}
}

func TestListenBackwardCompatibility(t *testing.T) {
	tests := []struct {
		name       string
		config     string
		wantAddrs  []string
		wantErr    bool
	}{
		{
			name: "single string (backward compat)",
			config: `[server]
listen = "127.0.0.1:5300"
[upstream]
servers = ["9.9.9.9:53"]
`,
			wantAddrs: []string{"127.0.0.1:5300"},
		},
		{
			name: "array with one address",
			config: `[server]
listen = ["127.0.0.1:5300"]
[upstream]
servers = ["9.9.9.9:53"]
`,
			wantAddrs: []string{"127.0.0.1:5300"},
		},
		{
			name: "array with multiple addresses",
			config: `[server]
listen = ["127.0.0.1:5300", "192.168.1.1:53"]
[upstream]
servers = ["9.9.9.9:53"]
`,
			wantAddrs: []string{"127.0.0.1:5300", "192.168.1.1:53"},
		},
		{
			name: "missing listen",
			config: `[server]
[upstream]
servers = ["9.9.9.9:53"]
`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "resolvit.conf")
			if err := os.WriteFile(configPath, []byte(tt.config), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("RESOLVIT_CONFIG", configPath)

			cfg, err := Setup()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(cfg.Server.Listen) != len(tt.wantAddrs) {
				t.Fatalf("got %d listen addresses, want %d", len(cfg.Server.Listen), len(tt.wantAddrs))
			}
			for i, addr := range tt.wantAddrs {
				if cfg.Server.Listen[i] != addr {
					t.Errorf("listen[%d] = %q, want %q", i, cfg.Server.Listen[i], addr)
				}
			}
		})
	}
}

func TestParseUpstream(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"8.8.8.8", "8.8.8.8:53"},
		{"8.8.8.8:5353", "8.8.8.8:5353"},
		{"1.1.1.1", "1.1.1.1:53"},
		{"9.9.9.9:53", "9.9.9.9:53"},
		{"8.8.4.4", "8.8.4.4:53"},
		{"208.67.222.222", "208.67.222.222:53"},
		{"208.67.222.222:5353", "208.67.222.222:5353"},
	}

	for _, tt := range tests {
		result := ParseUpstream(tt.input)
		if result != tt.expected {
			t.Errorf("ParseUpstream(%s) = %s, want %s", tt.input, result, tt.expected)
		}
	}
}
