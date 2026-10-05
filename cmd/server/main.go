package main

import (
	"github.com/pyromeowww/uptime-checker/internal/checker"
	"github.com/pyromeowww/uptime-checker/internal/config"
	"github.com/pyromeowww/uptime-checker/internal/server"
)

func main() {
	cfg := config.Load()
	chk := checker.NewChecker()
	server.RunServer(cfg, chk)
}
