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
    status TEXT NOT NULL DEFAULT 'PENDING',
    last_check TEXT,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP
    )`

	schemaCheckResults = `CREATE TABLE IF NOT EXISTS check_results (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    target_id INTEGER REFERENCES targets(id) ON DELETE CASCADE,
    status TEXT NOT NULL,
    status_code INTEGER,
    latency_ms TEXT,
    err_msg TEXT,
    checked_at TEXT DEFAULT CURRENT_TIMESTAMP
    )`

	schemaSubscriptions = `CREATE TABLE IF NOT EXISTS subscriptions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    target_id INTEGER NOT NULL,
    contact_info TEXT NOT NULL,
    period TEXT DEFAULT '1h',
    is_active INTEGER DEFAULT 1,
    created_at TEXT DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (target_id) REFERENCES targets(id) ON DELETE CASCADE,
    UNIQUE(target_id, contact_info, period)
    )`

	// Отдельный индекс для быстрого поиска истории по таргету
	indexCheckResults = `CREATE INDEX IF NOT EXISTS idx_check_results_target_id ON check_results(target_id);`

	// Отдельный индекс для быстрого поиска подписок по таргету (нужен воркеру)
	indexSubscriptions = `CREATE INDEX IF NOT EXISTS idx_subscriptions_target_id ON subscriptions(target_id);`
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

	schema := schemaTargets + ";" + schemaCheckResults + ";" + indexCheckResults + ";" + schemaSubscriptions + ";" + indexSubscriptions
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
