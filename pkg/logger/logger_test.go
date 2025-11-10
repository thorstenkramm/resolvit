package logger

import (
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestLogger(t *testing.T) {
	testCases := []struct {
		level    string
		message  string
		logFile  string
		wantText string
	}{
		{"debug", "debug message", "stdout", "debug message"},
		{"info", "info message", "stdout", "info message"},
		{"warn", "warn message", "stdout", "warn message"},
		{"error", "error message", "stdout", "error message"},
		{"debug", "debug to file", "test.log", "debug to file"},
		{"info", "info to file", "test.log", "info to file"},
		{"warn", "warn to file", "test.log", "warn to file"},
		{"error", "error to file", "test.log", "error to file"},
	}

	for _, tc := range testCases {
		t.Run(tc.level+"-"+tc.logFile, func(t *testing.T) {
			if tc.logFile != "stdout" {
				// Clean up any existing test log file
				if err := os.Remove(tc.logFile); err != nil && !os.IsNotExist(err) {
					t.Fatalf("os.Remove: %v", err)
				}
				defer func(filename string) {
					if err := os.Remove(filename); err != nil && !os.IsNotExist(err) {
						t.Errorf("os.Remove: %v", err)
					}
				}(tc.logFile)
			}

			Setup(tc.level, tc.logFile)

			// Log test message
			slog.Debug(tc.message)
			slog.Info(tc.message)
			slog.Warn(tc.message)
			slog.Error(tc.message)

			if tc.logFile == "stdout" {
				// For stdout tests, we can only verify setup completed without error
				return
			}

			// Read and verify log file content
			content, err := os.ReadFile(tc.logFile)
			if err != nil {
				t.Fatalf("Failed to read log file: %v", err)
			}

			logContent := string(content)
			if !strings.Contains(logContent, tc.wantText) {
				t.Errorf("Log file does not contain expected text %q", tc.wantText)
			}

			// Verify log level filtering
			switch tc.level {
			case "error":
				if strings.Contains(logContent, "level=INFO") {
					t.Error("Error level log contains INFO messages")
				}
			case "warn":
				if strings.Contains(logContent, "level=DEBUG") {
					t.Error("Warn level log contains DEBUG messages")
				}
			case "info":
				if strings.Contains(logContent, "level=DEBUG") {
					t.Error("Info level log contains DEBUG messages")
				}
			}
		})
	}
}
