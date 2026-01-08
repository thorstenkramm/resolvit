// Package main boots the resolvit DNS service.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"resolvit/pkg/config"
	"resolvit/pkg/filtering"
	"resolvit/pkg/logger"
	"resolvit/pkg/records"
	"resolvit/pkg/server"
	"resolvit/pkg/version"
	"syscall"
)

func main() {
	if handleVersionFlag() {
		return
	}
	if err := run(); err != nil {
		fmt.Printf("error: %v\n", err)
		os.Exit(1)
	}
}

func handleVersionFlag() bool {
	for _, arg := range os.Args[1:] {
		if arg == "--version" {
			fmt.Printf("resolvit version %s\n", version.ResolvitVersion)
			return true
		}
	}
	return false
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

	log := logger.Setup(cfg.Logging.Level, cfg.Logging.File)

	if err := loadRecords(cfg.Records.ResolveFrom, log); err != nil {
		return fmt.Errorf("load records: %w", err)
	}

	var filter *filtering.Filter
	if cfg.Filtering.Enabled {
		sources := filtering.BuildSources(filtering.Catalog, cfg.Filtering.Lists, cfg.Filtering.Custom.List)
		filter = filtering.NewFilter(filtering.FilterOptions{
			Enabled:         cfg.Filtering.Enabled,
			BlockSubdomains: cfg.Filtering.BlockSubdomains,
			AllowlistPath:   cfg.Filtering.Allowlist.Path,
			Sources:         sources,
			CacheDir:        cfg.Filtering.CacheDir,
			UpdateInterval:  cfg.Filtering.UpdateInterval,
			BlockedLogPath:  cfg.Filtering.BlockedLog,
			Log:             log,
			ErrorLimit:      cfg.Logging.BlocklistErrorLimit,
		})
		filter.Start(context.Background())
	}

	srv := server.New(cfg.Server.Listen, cfg.Upstream.Servers, log, filter)
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
			if err := loadRecords(cfg.Records.ResolveFrom, log); err != nil {
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
