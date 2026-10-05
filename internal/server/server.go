package server

import (
	"log"
	"net/http"
	"os"
	"time"

	"github.com/pyromeowww/uptime-checker/internal/checker"
	"github.com/pyromeowww/uptime-checker/internal/config"
)

func RunServer(cfg *config.Config, chk *checker.Checker) {
	logger := log.New(os.Stdout, "[server]", log.LstdFlags|log.Lshortfile)

	mux := http.NewServeMux()
	h := NewHandler(chk)
	mux.HandleFunc("/check", h.CheckHandler)

	server := &http.Server{
		Addr:         cfg.Port,
		Handler:      mux,
		ErrorLog:     logger,
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  15 * time.Second,
	}

	logger.Printf("The server is running. Port: %s", cfg.Port)

	err := server.ListenAndServe()
	if err != nil {
		logger.Fatal("Server startup error: ", err)
	}
}
