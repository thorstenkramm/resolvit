// Package logger configures slog for the resolvit server.
package logger

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// Setup builds a slog.Logger using the requested level and writer destination.
func Setup(logLevel string, logFile string) *slog.Logger {
	var logWriter = os.Stdout
	var handlerOptions = &slog.HandlerOptions{Level: getLogLevel(logLevel)}

	if logFile != "stdout" {
		resolvedPath, err := sanitizeLogPath(logFile)
		if err != nil {
			slog.Error("invalid log file path", "path", logFile, "error", err)
			os.Exit(1)
		}
		// #nosec G304 -- path validated via sanitizeLogPath
		logWriter, err = os.OpenFile(
			resolvedPath,
			os.O_APPEND|os.O_CREATE|os.O_WRONLY,
			0o600,
		)
		if err != nil {
			slog.Error("failed to open log file", "path", resolvedPath, "error", err)
			os.Exit(1)
		}
	} else {
		// Configure handler to remove the time key if writing to stdout
		handlerOptions.ReplaceAttr = func(_ []string, attr slog.Attr) slog.Attr {
			if attr.Key == slog.TimeKey {
				return slog.Attr{} // Return an empty Attr to remove it
			}
			return attr
		}
	}

	logger := slog.New(slog.NewTextHandler(logWriter, handlerOptions))
	slog.SetDefault(logger)
	return logger
}

func getLogLevel(logLevel string) slog.Level {
	var level slog.Level
	switch strings.ToLower(logLevel) {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	return level
}

// sanitizeLogPath rejects empty, root, or parent paths to avoid traversal.
func sanitizeLogPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("log file path is empty")
	}

	clean := filepath.Clean(path)
	if clean == "." || clean == string(os.PathSeparator) {
		return "", fmt.Errorf("log file path %q resolves to a directory", path)
	}

	if clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("log file path %q escapes the working directory", path)
	}

	return clean, nil
}
