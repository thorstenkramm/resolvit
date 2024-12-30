package main

import (
	"fmt"
	log2 "log"
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
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
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

	go func() {
		srv := server.New(cfg.Listen, cfg.Upstreams, log)
		if err := srv.Start(); err != nil {
			log2.Fatalf("start server: %s", err)
		}
	}()

	sig := make(chan os.Signal)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	for {
		s := <-sig
		switch s {
		case syscall.SIGINT:
			log2.Printf("receive SIGINT, shutting down")
			os.Exit(0)
		case syscall.SIGHUP:
			log2.Printf("receive SIGHUP, reloading records")
			if err := loadRecords(cfg.ResolveFrom, log); err != nil {
				log2.Printf("failed to reload records: %v", err)
			} else {
				log2.Printf("records reloaded successfully")
			}
			continue
		default:
			log2.Fatalf("Signal (%v) received, stopping\n", s)
		}
	}
}
