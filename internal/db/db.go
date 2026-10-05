package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pyromeowww/uptime-checker/internal/config"
)

const (
	schemaTargets = `CREATE TABLE IF NOT EXISTS targets (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	url TEXT NOT NULL UNIQUE,
	status TEXT NOT NULL,
	last_check DATETIME,
	created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`
	schemaCheck_Results = `CREATE TABLE IF NOT EXISTS check_results (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	target_id INTEGER REFERENCES targets(id) ON DELETE CASCADE,
	status TEXT NOT NULL,
	status_code INTEGER,
	latency_ms INTEGER,
	err_msg TEXT,
	checked_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`
	indexCheck = `CREATE INDEX IF NOT EXISTS idx_check_results_target_id ON check_results(target_id);`
)

var DB *sql.DB

func Close() error {
	if DB != nil {
		return DB.Close()
	}
	return nil
}

func Init(cfg *config.Config) (err error) {
	dir := filepath.Dir(cfg.DBFile)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create db directory %q: %w", dir, err)
		}
	}

	info, err := os.Stat(cfg.DBFile)
	switch {
	case err == nil:
		if info.IsDir() {
			return fmt.Errorf("db path %q is a directory, not a file", cfg.DBFile)
		}
	default:
		return err
	}

	DB, err = sql.Open("sqlite", cfg.DBFile)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}
	if err = DB.Ping(); err != nil {
		return fmt.Errorf("failed to ping database: %w", err)
	}

	if _, err := DB.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		return fmt.Errorf("failed to enable foreign keys: %w", err)
	}
	DB.SetMaxOpenConns(1)

	schema := schemaTargets + ";" + schemaCheck_Results + ";" + indexCheck
	if _, err := DB.Exec(schema); err != nil {
		return fmt.Errorf("failed to initialize schema: %w", err)
	}

	return nil
}
