package db

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/pyromeowww/uptime-checker/internal/domain"
)

func TestGetOrCreateTarget(t *testing.T) {

	t.Run("new url", func(t *testing.T) {
		ctx := context.Background()
		url := "https://github.com"
		store, db := newTestStore(t)
		id, err := store.GetOrCreateTarget(ctx, url)
		if err != nil {
			t.Fatalf("не ожидали ошибки, но получили: %v", err)
		}
		if id <= 0 {
			t.Fatalf("ожидали положительный ID, получили: %d", id)
		}

		var count int
		err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM targets WHERE url = ?", url).Scan(&count)
		if err != nil {
			t.Fatalf("не удалось выполнить запрос: %v", err)
		}
		if count != 1 {
			t.Fatalf("ожидали 1 строку в targets, получили: %d", count)
		}

		var status string
		err = db.QueryRowContext(ctx, "SELECT status FROM targets WHERE url = ?", url).Scan(&status)
		if err != nil {
			t.Fatalf("не удалось выполнить запрос: %v", err)
		}
		if status != string(domain.StatusPending) {
			t.Fatalf("ожидали статус %q, получили: %v", domain.StatusPending, status)
		}
	})

	t.Run("existing url", func(t *testing.T) {
		ctx := context.Background()
		url := "https://google.com"
		store, db := newTestStore(t)
		id1, err := store.GetOrCreateTarget(ctx, url)
		if err != nil {
			t.Fatalf("не ожидали ошибки, но получили: %v", err)
		}
		id2, err := store.GetOrCreateTarget(ctx, url)
		if err != nil {
			t.Fatalf("не ожидали ошибки, но получили: %v", err)
		}

		if id1 <= 0 {
			t.Fatalf("ожидали положительный ID, получили: %d", id1)
		}
		if id2 <= 0 {
			t.Fatalf("ожидали положительный ID, получили: %d", id2)
		}

		if id1 != id2 {
			t.Fatalf("ожидали id1 = id2, получили id1 = %d, id2 = %d", id1, id2)
		}

		var count int
		query := `SELECT COUNT(*) FROM targets WHERE url = ?`
		err = db.QueryRowContext(ctx, query, url).Scan(&count)
		if err != nil {
			t.Fatalf("не удалось выполнить запрос: %v", err)
		}
		if count != 1 {
			t.Fatalf("ожидали 1 строку в targets, получили: %d", count)
		}
	})

}

func TestSaveResult_Success(t *testing.T) {
	ctx := context.Background()
	url := "https://docker.com"
	store, db := newTestStore(t)
	id, err := store.GetOrCreateTarget(ctx, url)
	if err != nil {
		t.Fatalf("не ожидали ошибку, но получили: %v", err)
	}

	res := domain.CheckResult{
		TargetID:   id,
		Status:     domain.StatusUp,
		StatusCode: 200,
		LatencyMs:  "123",
		ErrMsg:     "",
		CheckedAt:  "2026-10-09 07:02:00",
	}

	err = store.SaveResult(ctx, res)
	if err != nil {
		t.Fatalf("не ожидали ошибку при SaveResult, но получили: %v", err)
	}
	assertCheckResults(t, db, ctx, id, res)
	assertTargets(t, db, ctx, id, res)
}

func newTestStore(t *testing.T) (*Store, *sql.DB) {
	path := filepath.Join(t.TempDir(), "test.db")
	dsn := fmt.Sprintf("%s?mode=rwc", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatalf("ошибка открытия базы данных: %v", err)
	}

	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("ошибка при закрытии тестовой базы данных: %v", err)
		}
	})

	if err = db.Ping(); err != nil {
		t.Fatalf("failed to ping database: %v", err)
	}

	db.SetMaxOpenConns(1)
	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		t.Fatalf("failed to enable foreign keys: %v", err)
	}

	schema := schemaTargets + ";" + schemaCheck_Results + ";" + indexCheck
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("ошибка создания таблиц: %v", err)
	}

	return &Store{db: db}, db
}

func assertCheckResults(t *testing.T, db *sql.DB, ctx context.Context, targetID int64, want domain.CheckResult) {
	t.Helper()
	var (
		gotID, gotTargetID                               int64
		gotStatusCode                                    int
		gotStatus, gotLatencyMs, gotErrMsg, gotCheckedAt string
	)

	query := `SELECT id, target_id, status, status_code, latency_ms, err_msg, checked_at FROM check_results WHERE target_id = ?`
	err := db.QueryRowContext(ctx, query, targetID).Scan(&gotID, &gotTargetID, &gotStatus, &gotStatusCode, &gotLatencyMs, &gotErrMsg, &gotCheckedAt)
	if err != nil {
		t.Fatalf("не удалось выполнить запрос: %v", err)
	}
	if gotID <= 0 {
		t.Fatalf("ожидали положительный ID, получили: %d", gotID)
	}
	if gotTargetID != want.TargetID {
		t.Fatalf("target_id в check_results: ожидали %d, получили: %d", want.TargetID, gotTargetID)
	}
	if gotStatus != string(want.Status) {
		t.Fatalf("status в check_results: ожидали %q, получили: %q", want.Status, gotStatus)
	}
	if gotStatusCode != want.StatusCode {
		t.Fatalf("status_code в check_results: ожидали %d, получили: %d", want.StatusCode, gotStatusCode)
	}
	if gotLatencyMs != want.LatencyMs {
		t.Fatalf("latency_ms в check_results: ожидали %q, получили: %q", want.LatencyMs, gotLatencyMs)
	}
	if gotErrMsg != want.ErrMsg {
		t.Fatalf("err_msg в check_results: ожидали %q, получили: %q", want.ErrMsg, gotErrMsg)
	}

	wantTime, err := time.Parse("2006-01-02 15:04:05", want.CheckedAt)
	if err != nil {
		t.Fatalf("check_results: не удалось распарсить want.CheckedAt: %v", err)
	}
	got, err := time.Parse(time.RFC3339, gotCheckedAt)
	if err != nil {
		t.Fatalf("check_results: не удалось распарсить gotCheckedAt: %v", err)
	}
	if !wantTime.Equal(got) {
		t.Fatalf("checked_at в check_results: ожидали %v, получили: %v", wantTime.String(), got.String())
	}
}

func assertTargets(t *testing.T, db *sql.DB, ctx context.Context, id int64, res domain.CheckResult) {
	t.Helper()
	var (
		gotStatus, gotLastCheck string
	)
	query := `SELECT status, last_check FROM targets WHERE id = ?`
	err := db.QueryRowContext(ctx, query, id).Scan(&gotStatus, &gotLastCheck)
	if err != nil {
		t.Fatalf("targets: не удалось выполнить запрос: %v", err)
	}
	if gotStatus != string(res.Status) {
		t.Fatalf("несовпадение статуса для target_id= %d: ожидали %q, получили %q", id, res.Status, gotStatus)
	}

	wantTime, err := time.Parse("2006-01-02 15:04:05", res.CheckedAt)
	if err != nil {
		t.Fatalf("targets: не удалось распарсить res.CheckedAt: %v", err)
	}
	gotTime, err := time.Parse(time.RFC3339, gotLastCheck)
	if err != nil {
		t.Fatalf("targets: не удалось распарсить gotLastCheck: %v", err)
	}
	if !wantTime.Equal(gotTime) {
		t.Fatalf("несовпадение last_check для target_id= %d: ожидали %q, получили %q", id, res.CheckedAt, gotLastCheck)
	}
}
