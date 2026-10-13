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

	// 1. Инициализируем хранилище базы данных
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

	// 2. Запускаем сервер, передавая ему store для работы с БД
	server.RunServer(cfg, chk, store)
}
