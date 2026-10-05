package config

import (
	"log"
	"os"
	"strconv"
)

type Config struct {
	Port   string
	DBFile string
}

func Load() *Config {
	return &Config{
		Port:   getPortEnv("TODO_PORT", ":8080"),
		DBFile: getDBEnv("TODO_DB", "uptime.db"),
	}
}

// getPortEnv считывает и валидирует порт из переменной окружения
func getPortEnv(port, defaultPort string) string {
	val := os.Getenv(port)
	if val == "" {
		return defaultPort
	}

	v, err := strconv.Atoi(val)
	if err != nil || v < 1 || v > 65535 {
		log.Fatalf("invalid %s=%q: must be integer 1..65535", port, val)
	}

	return ":" + val
}

func getDBEnv(dbFile, defaultDB string) string {
	val := os.Getenv(dbFile)
	if val == "" {
		return defaultDB
	}
	return val
}
