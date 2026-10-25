package server

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/pyromeowww/uptime-checker/internal/checker"
	"github.com/pyromeowww/uptime-checker/internal/config"
	"github.com/pyromeowww/uptime-checker/internal/db"
)

func RunServer(ctx context.Context, cfg *config.Config, chk *checker.Checker, store *db.Store) error {
	logger := log.New(os.Stdout, "[server]", log.LstdFlags|log.Lshortfile)
	startCleanupWorker(ctx, store, 24*time.Hour, 48*time.Hour)

	mux := http.NewServeMux()
	h := NewHandler(chk, store)
	mux.HandleFunc("POST /api/check", h.CheckHandler)
	mux.HandleFunc("POST /api/subscriptions", h.CreateTargetHandler)
	mux.HandleFunc("GET /api/history", h.GetHistoryHandler)
	mux.HandleFunc("GET /api/history/all", h.GetAllHistoryHandler)
	mux.HandleFunc("GET /api/subscriptions/all", h.GetAllSubscriptionsHandler)
	mux.HandleFunc("DELETE /api/subscriptions/all", h.DeleteAllSubscriptionsHandler)
	mux.HandleFunc("DELETE /api/subscriptions", h.DeleteSubscriptionsHandler)

	server := &http.Server{
		Addr:         cfg.Port,
		Handler:      mux,
		ErrorLog:     logger,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 45 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		<-ctx.Done()
		logger.Printf("Shutting down server...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Printf("Server shutdown error: %v", err)
		}
	}()

	logger.Printf("The server is running. Port: %s", cfg.Port)

	err := server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func startCleanupWorker(ctx context.Context, store *db.Store, interval, retentionPeriod time.Duration) {
	go func() {
		logger := log.New(os.Stdout, "[server]", log.LstdFlags|log.Lshortfile)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				logger.Printf("Cleanup worker stopped")
				return
			case <-ticker.C:
				cleanupCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				deletedCount, err := store.DeleteOldResults(cleanupCtx, retentionPeriod)
				cancel()
				if err != nil {
					logger.Printf("Failed to delete old check results: %v", err)
				} else if deletedCount > 0 {
					logger.Printf("Background cleanup: deleted %d old check results", deletedCount)
				}
			}
		}
	}()
}
