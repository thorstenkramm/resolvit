package config

import (
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
