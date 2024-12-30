package main

import (
	"context"
	"os"
	"os/signal"
	"resolvit/pkg/config"
	"resolvit/pkg/logger"
	"resolvit/pkg/records"
	"resolvit/pkg/server"
	"syscall"
	"time"
)

func main() {
	cfg, err := config.Setup()
	if err != nil {
		os.Exit(1)
	}

	log := logger.Setup(cfg.LogLevel, cfg.LogFile)

	if cfg.ResolveFrom != "" {
		if err := records.LoadFromFile(cfg.ResolveFrom, log); err != nil {
			log.Error("failed to load records file", "error", err)
			os.Exit(1)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	srv := server.New(cfg.Listen, cfg.Upstreams, log)
	if err := srv.Start(ctx); err != nil {
		log.Error("failed to start server", "error", err)
		os.Exit(1)
	}

	// Set up signal handling
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	for {
		sig := <-sigChan
		switch sig {
		case syscall.SIGHUP:
			log.Info("received SIGHUP signal, reloading local records")
			if cfg.ResolveFrom != "" {
				if err := records.LoadFromFile(cfg.ResolveFrom, log); err != nil {
					log.Error("failed to reload records file", "error", err)
				} else {
					log.Info("successfully reloaded local records")
				}
			}
		case syscall.SIGINT, syscall.SIGTERM:
			log.Info("received shutdown signal", "signal", sig)

			// First cancel the context to stop workers
			cancel()

			// Then shutdown the server
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()

			if err := srv.Shutdown(shutdownCtx); err != nil {
				log.Error("shutdown failed", "error", err)
				os.Exit(1)
			}
			return
		}
	}
}
