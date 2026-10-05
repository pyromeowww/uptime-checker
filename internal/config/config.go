package config

import (
	"log"
	"os"
	"strconv"
)

type Config struct {
	Port        string
	HTTPTimeout string
}

func Load() *Config {
	return &Config{
		Port: getPortEnv("TODO_PORT", ":8080"),
	}
}

// getPortEnv считывает и валидирует порт из переменной окружения
func getPortEnv(key, defaultPort string) string {
	val := os.Getenv(key)
	if val == "" {
		return defaultPort
	}

	v, err := strconv.Atoi(val)
	if err != nil || v < 1 || v > 65535 {
		log.Fatalf("invalid %s=%q: must be integer 1..65535", key, val)
	}

	return ":" + val
}
