package server

import (
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

	mux := http.NewServeMux()
	h := NewHandler(chk, store) // Передаем store в хэндлер
	mux.HandleFunc("/check", h.CheckHandler)

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
