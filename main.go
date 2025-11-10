// Package main boots the resolvit DNS service.
package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"resolvit/pkg/config"
	"resolvit/pkg/logger"
	"resolvit/pkg/records"
	"resolvit/pkg/server"
	"syscall"
)

func main() {
	if err := run(); err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
}

func loadRecords(recordsPath string, log *slog.Logger) error {
	if recordsPath == "" {
		return nil
	}
	return records.LoadFromFile(recordsPath, log)
}

func run() error {
	cfg, err := config.Setup()
	if err != nil {
		return fmt.Errorf("setup config: %w", err)
	}

	log := logger.Setup(cfg.LogLevel, cfg.LogFile)

	if err := loadRecords(cfg.ResolveFrom, log); err != nil {
		return fmt.Errorf("load records: %w", err)
	}

	srv := server.New(cfg.Listen, cfg.Upstreams, log)
	go func() {
		if err := srv.Start(); err != nil {
			log.Error("failed to start server", "error", err)
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	for {
		s := <-sig
		switch s {
		case syscall.SIGINT, syscall.SIGTERM:
			log.Info("shutting down")
			os.Exit(0)
		case syscall.SIGHUP:
			log.Info("receive SIGHUP, reloading records")
			if err := loadRecords(cfg.ResolveFrom, log); err != nil {
				log.Error("failed to reload records:", "error", err)
			} else {
				srv.ClearCache()
				log.Info("records reloaded successfully")
			}
			continue
		default:
			log.Error("Unknown signal received, stopping", "signal", s)
			os.Exit(1)
		}
	}
}
