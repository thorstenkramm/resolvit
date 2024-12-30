package logger

import (
	"log/slog"
	"os"
	"strings"
)

func Setup(logLevel string, logFile string) *slog.Logger {
	var logWriter = os.Stdout
	var handlerOptions = &slog.HandlerOptions{Level: getLogLevel(logLevel)}

	//if logFile != "stdout" {
	//	var err error
	//	logWriter, err = os.OpenFile(logFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	//	if err != nil {
	//		slog.Error("failed to open log file", "error", err)
	//		os.Exit(1)
	//	}
	//} else {
	//	// Configure handler to remove the time key if writing to stdout
	//	handlerOptions.ReplaceAttr = func(_ []string, attr slog.Attr) slog.Attr {
	//		if attr.Key == slog.TimeKey {
	//			return slog.Attr{} // Return an empty Attr to remove it
	//		}
	//		return attr
	//	}
	//}

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
