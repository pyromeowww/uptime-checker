package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pyromeowww/uptime-checker/internal/config"
	_ "modernc.org/sqlite"
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

type Store struct {
	db *sql.DB
}

// New инициализирует подключение и возвращает экземпляр *Store
func New(cfg *config.Config) (*Store, error) {
	dir := filepath.Dir(cfg.DBFile)
	if dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create db directory %q: %w", dir, err)
		}
	}

	dsn := fmt.Sprintf("%s?mode=rwc", cfg.DBFile)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	var success bool
	defer func() {
		if !success {
			_ = db.Close() // Закрываем, если инициализация не дошла до конца
		}
	}()

	if err = db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	schema := schemaTargets + ";" + schemaCheck_Results + ";" + indexCheck
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	success = true

	return &Store{db: db}, nil
}

// Close закрывает подключение к базе данных
func (s *Store) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}
