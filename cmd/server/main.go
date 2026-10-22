package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pyromeowww/uptime-checker/internal/checker"
	"github.com/pyromeowww/uptime-checker/internal/config"
	"github.com/pyromeowww/uptime-checker/internal/db"
	"github.com/pyromeowww/uptime-checker/internal/server"
	"github.com/pyromeowww/uptime-checker/internal/worker"
)

func main() {
	cfg := config.Load()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		log.Printf("Received signal %v, shutting down...", sig)
		cancel()
	}()

	store, err := db.New(cfg)
	if err != nil {
		log.Fatalf("Database initialization error: %v", err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			log.Printf("error closing database: %v", err)
		}
	}()

	chk := checker.NewChecker()

	chkWorker := worker.NewWorker(store, 1*time.Minute)
	go chkWorker.Start(ctx)
	log.Println("Background worker started")

	if err := server.RunServer(ctx, cfg, chk, store); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("Server error: %v", err)
	}
	log.Println("Shutdown complete")
}
