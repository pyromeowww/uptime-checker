package main

import (
	"log"

	"github.com/pyromeowww/uptime-checker/internal/checker"
	"github.com/pyromeowww/uptime-checker/internal/config"
	"github.com/pyromeowww/uptime-checker/internal/db"
	"github.com/pyromeowww/uptime-checker/internal/server"
)

func main() {
	cfg := config.Load()
	chk := checker.NewChecker()
	if err := db.Init(cfg); err != nil {
		log.Fatalf("Database initialization error: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("error closing database: %v", err)
		}
	}()
	server.RunServer(cfg, chk)
}
