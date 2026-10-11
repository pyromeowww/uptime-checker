package server

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/pyromeowww/uptime-checker/internal/checker"
	"github.com/pyromeowww/uptime-checker/internal/config"
	"github.com/pyromeowww/uptime-checker/internal/db" // Импортируем пакет db
)

func RunServer(cfg *config.Config, chk *checker.Checker, store *db.Store) {
	logger := log.New(os.Stdout, "[server]", log.LstdFlags|log.Lshortfile)
	startCleanupWorker(store, 24*time.Hour, 48*time.Hour)

	mux := http.NewServeMux()
	h := NewHandler(chk, store) // Передаем store в хэндлер
	mux.HandleFunc("POST /check", h.CheckHandler)
	mux.HandleFunc("GET /api/history", h.GetHistoryHandler)
	mux.HandleFunc("GET /api/history/all", h.GetAllHistoryHandler)

	server := &http.Server{
		Addr:         cfg.Port,
		Handler:      mux,
		ErrorLog:     logger,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	logger.Printf("The server is running. Port: %s", cfg.Port)

	err := server.ListenAndServe()
	if err != nil {
		logger.Fatal("Server startup error: ", err)
	}
}

func startCleanupWorker(store *db.Store, interval time.Duration, retentionPeriod time.Duration) {
	go func() {
		logger := log.New(os.Stdout, "[server]", log.LstdFlags|log.Lshortfile)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				deletedCount, err := store.DeleteOldResults(ctx, retentionPeriod)
				cancel()
				if err != nil {
					logger.Printf("Failed to delete old check results: %v", err)
				} else if deletedCount > 0 {
					logger.Printf("Background cleanup: successfully deleted %d old check results", deletedCount)
				}
			}
		}
	}()
}
