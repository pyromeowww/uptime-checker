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

const timeLayout = "2006-01-02 15:04:05"

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
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM targets WHERE url = ?", url).Scan(&count); err != nil {
			t.Fatalf("не удалось выполнить запрос: %v", err)
		}
		if count != 1 {
			t.Fatalf("ожидали 1 строку в targets, получили: %d", count)
		}

		var status string
		if err := db.QueryRowContext(ctx, "SELECT status FROM targets WHERE url = ?", url).Scan(&status); err != nil {
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
		if id1 != id2 {
			t.Fatalf("ожидали id1 = id2, получили id1 = %d, id2 = %d", id1, id2)
		}

		var count int
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM targets WHERE url = ?`, url).Scan(&count); err != nil {
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
		CheckedAt:  time.Now().UTC().Add(-time.Hour).Format(timeLayout),
	}

	if err := store.SaveResult(ctx, res); err != nil {
		t.Fatalf("не ожидали ошибку при SaveResult, но получили: %v", err)
	}
	assertCheckResults(t, db, ctx, id, res)
	assertTargets(t, db, ctx, id, res)
}

func TestStore_GetHistory(t *testing.T) {
	ctx := context.Background()
	url := "https://docker.com"
	store, _ := newTestStore(t)
	id, err := store.GetOrCreateTarget(ctx, url)
	if err != nil {
		t.Fatalf("не ожидали ошибку, но получили: %v", err)
	}

	now := time.Now().UTC()
	res1 := domain.CheckResult{
		TargetID: id, Status: domain.StatusUp, StatusCode: 200, LatencyMs: "123",
		CheckedAt: now.Add(-2 * time.Hour).Format(timeLayout),
	}
	if err := store.SaveResult(ctx, res1); err != nil {
		t.Fatalf("не ожидали ошибку при SaveResult, но получили: %v", err)
	}
	res2 := domain.CheckResult{
		TargetID: id, Status: domain.StatusUp, StatusCode: 200, LatencyMs: "123",
		CheckedAt: now.Add(-1 * time.Hour).Format(timeLayout),
	}
	if err := store.SaveResult(ctx, res2); err != nil {
		t.Fatalf("не ожидали ошибку при SaveResult, но получили: %v", err)
	}

	period := 72 * time.Hour

	t.Run("GetHistoryByURL", func(t *testing.T) {
		history, err := store.GetHistoryByURL(ctx, url, period)
		if err != nil {
			t.Fatalf("не ожидали ошибку, но получили: %v", err)
		}
		if len(history) != 2 {
			t.Errorf("ожидали 2 записи в истории, а получили %d", len(history))
		}
	})

	t.Run("GetTargetHistoryById", func(t *testing.T) {
		history, err := store.GetTargetHistory(ctx, id, period)
		if err != nil {
			t.Fatalf("не ожидали ошибку, но получили: %v", err)
		}
		if len(history) != 2 {
			t.Errorf("ожидали 2 записи в истории, а получили %d", len(history))
		}
	})

	t.Run("GetAllHistory", func(t *testing.T) {
		history, err := store.GetAllHistory(ctx, period)
		if err != nil {
			t.Fatalf("не ожидали ошибку, но получили: %v", err)
		}
		if len(history) != 2 {
			t.Errorf("ожидали 2 записи в истории, а получили %d", len(history))
		}
	})
}

func TestStore_GetAllSubscriptions(t *testing.T) {
	ctx := context.Background()

	t.Run("empty", func(t *testing.T) {
		store, _ := newTestStore(t)

		subs, err := store.GetAllSubscriptions(ctx, 100, 0)
		if err != nil {
			t.Fatalf("не ожидали ошибку: %v", err)
		}
		if len(subs) != 0 {
			t.Fatalf("ожидали 0 подписок, получили %d", len(subs))
		}
		if subs == nil {
			t.Fatal("ожидали пустой слайс, а не nil")
		}
	})

	t.Run("after create", func(t *testing.T) {
		store, _ := newTestStore(t)

		if _, err := store.CreateSubscriptions(ctx, domain.SubscriptionRequest{
			URLs:        []string{"https://a.example", "https://b.example"},
			ContactInfo: "tg:@user",
			Period:      "5m",
		}); err != nil {
			t.Fatalf("подготовка: %v", err)
		}

		subs, err := store.GetAllSubscriptions(ctx, 100, 0)
		if err != nil {
			t.Fatalf("не ожидали ошибку: %v", err)
		}
		if len(subs) != 2 {
			t.Fatalf("ожидали 2 подписки, получили %d", len(subs))
		}
		if subs[0].ID >= subs[1].ID {
			t.Fatalf("ожидали порядок по возрастанию id, получили %d, %d", subs[0].ID, subs[1].ID)
		}
		if subs[0].URL != "https://a.example" {
			t.Errorf("URL: ожидали %q, получили %q", "https://a.example", subs[0].URL)
		}
		if subs[1].URL != "https://b.example" {
			t.Errorf("URL: ожидали %q, получили %q", "https://b.example", subs[1].URL)
		}
	})

	t.Run("pagination", func(t *testing.T) {
		store, _ := newTestStore(t)

		// 3 подписки, чтобы проверить limit=2, offset=0 / limit=2, offset=2
		for _, u := range []string{"https://a.example", "https://b.example", "https://c.example"} {
			if _, err := store.CreateSubscriptions(ctx, domain.SubscriptionRequest{
				URLs:        []string{u},
				ContactInfo: "tg:@user",
				Period:      "5m",
			}); err != nil {
				t.Fatalf("подготовка %s: %v", u, err)
			}
		}

		page1, err := store.GetAllSubscriptions(ctx, 2, 0)
		if err != nil {
			t.Fatalf("page1: %v", err)
		}
		if len(page1) != 2 {
			t.Fatalf("page1: ожидали 2, получили %d", len(page1))
		}

		page2, err := store.GetAllSubscriptions(ctx, 2, 2)
		if err != nil {
			t.Fatalf("page2: %v", err)
		}
		if len(page2) != 1 {
			t.Fatalf("page2: ожидали 1, получили %d", len(page2))
		}

		// страницы не пересекаются
		if page1[0].ID == page2[0].ID || page1[1].ID == page2[0].ID {
			t.Fatalf("страницы пересекаются: page1 ids %d,%d, page2 id %d",
				page1[0].ID, page1[1].ID, page2[0].ID)
		}

		// offset за пределами — пустой слайс, не nil, не ошибка
		empty, err := store.GetAllSubscriptions(ctx, 2, 100)
		if err != nil {
			t.Fatalf("offset за пределами: %v", err)
		}
		if len(empty) != 0 {
			t.Fatalf("ожидали 0, получили %d", len(empty))
		}
		if empty == nil {
			t.Fatal("ожидали пустой слайс, а не nil")
		}
	})
}

func TestStore_DeleteAllSubscriptions(t *testing.T) {
	ctx := context.Background()
	store, db := newTestStore(t)

	if _, err := store.CreateSubscriptions(ctx, domain.SubscriptionRequest{
		URLs:        []string{"https://a.example", "https://b.example"},
		ContactInfo: "tg:@user",
		Period:      "5m",
	}); err != nil {
		t.Fatalf("подготовка: %v", err)
	}

	if _, err := store.DeleteAllSubscriptions(ctx); err != nil {
		t.Fatalf("DeleteAllSubscriptions: %v", err)
	}

	var subs, targets int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM subscriptions`).Scan(&subs); err != nil {
		t.Fatalf("count subscriptions: %v", err)
	}
	if subs != 0 {
		t.Fatalf("ожидали 0 подписок, получили %d", subs)
	}
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM targets`).Scan(&targets); err != nil {
		t.Fatalf("count targets: %v", err)
	}
	if targets != 2 {
		t.Fatalf("targets не должны удаляться: ожидали 2, получили %d", targets)
	}
}

func TestStore_DeleteSubscriptionsByURLs(t *testing.T) {
	ctx := context.Background()
	store, db := newTestStore(t)

	if _, err := store.CreateSubscriptions(ctx, domain.SubscriptionRequest{
		URLs:        []string{"https://a.example", "https://b.example", "https://c.example", "https://d.example"},
		ContactInfo: "tg:@user",
		Period:      "5m",
	}); err != nil {
		t.Fatalf("подготовка: %v", err)
	}

	// удаляем 3 из 4
	n, err := store.DeleteSubscriptionsByURLs(ctx, []string{
		"https://a.example",
		"https://c.example",
		"https://d.example",
	})
	if err != nil {
		t.Fatalf("DeleteSubscriptionsByURLs: %v", err)
	}
	if n != 3 {
		t.Fatalf("ожидали 3 удалённые строки, получили %d", n)
	}

	// осталась одна
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM subscriptions`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("ожидали 1 подписку, получили %d", count)
	}

	// повторное удаление — 0, не ошибка
	n, err = store.DeleteSubscriptionsByURLs(ctx, []string{"https://a.example"})
	if err != nil {
		t.Fatalf("повторное удаление: %v", err)
	}
	if n != 0 {
		t.Fatalf("ожидали 0, получили %d", n)
	}

	// пустой список — 0, не ошибка
	n, err = store.DeleteSubscriptionsByURLs(ctx, nil)
	if err != nil {
		t.Fatalf("пустой список: %v", err)
	}
	if n != 0 {
		t.Fatalf("ожидали 0, получили %d", n)
	}

	// targets не тронуты
	var targets int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM targets`).Scan(&targets); err != nil {
		t.Fatalf("count targets: %v", err)
	}
	if targets != 4 {
		t.Fatalf("targets не должны удаляться: ожидали 4, получили %d", targets)
	}
}

func newTestStore(t *testing.T) (*Store, *sql.DB) {
	t.Helper()
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

	db.SetMaxOpenConns(1)

	if err = db.Ping(); err != nil {
		t.Fatalf("failed to ping database: %v", err)
	}
	if _, err := db.Exec("PRAGMA foreign_keys = ON;"); err != nil {
		t.Fatalf("failed to enable foreign keys: %v", err)
	}

	if _, err := db.Exec(fullSchema()); err != nil {
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
	if err := db.QueryRowContext(ctx, query, targetID).
		Scan(&gotID, &gotTargetID, &gotStatus, &gotStatusCode, &gotLatencyMs, &gotErrMsg, &gotCheckedAt); err != nil {
		t.Fatalf("не удалось выполнить запрос: %v", err)
	}
	if gotID <= 0 {
		t.Fatalf("ожидали положительный ID, получили: %d", gotID)
	}
	if gotTargetID != want.TargetID {
		t.Fatalf("target_id: ожидали %d, получили: %d", want.TargetID, gotTargetID)
	}
	if gotStatus != string(want.Status) {
		t.Fatalf("status: ожидали %q, получили: %q", want.Status, gotStatus)
	}
	if gotStatusCode != want.StatusCode {
		t.Fatalf("status_code: ожидали %d, получили: %d", want.StatusCode, gotStatusCode)
	}
	if gotLatencyMs != want.LatencyMs {
		t.Fatalf("latency_ms: ожидали %q, получили: %q", want.LatencyMs, gotLatencyMs)
	}
	if gotErrMsg != want.ErrMsg {
		t.Fatalf("err_msg: ожидали %q, получили: %q", want.ErrMsg, gotErrMsg)
	}

	wantTime, err := time.Parse(timeLayout, want.CheckedAt)
	if err != nil {
		t.Fatalf("не удалось распарсить want.CheckedAt: %v", err)
	}
	gotTime, err := time.Parse(timeLayout, gotCheckedAt)
	if err != nil {
		t.Fatalf("не удалось распарсить gotCheckedAt %q: %v", gotCheckedAt, err)
	}
	if !wantTime.Equal(gotTime) {
		t.Fatalf("checked_at: ожидали %v, получили: %v", wantTime, gotTime)
	}
}

func assertTargets(t *testing.T, db *sql.DB, ctx context.Context, id int64, res domain.CheckResult) {
	t.Helper()
	var gotStatus, gotLastCheck string
	query := `SELECT status, last_check FROM targets WHERE id = ?`
	if err := db.QueryRowContext(ctx, query, id).Scan(&gotStatus, &gotLastCheck); err != nil {
		t.Fatalf("targets: не удалось выполнить запрос: %v", err)
	}
	if gotStatus != string(res.Status) {
		t.Fatalf("несовпадение статуса для target_id=%d: ожидали %q, получили %q", id, res.Status, gotStatus)
	}

	wantTime, err := time.Parse(timeLayout, res.CheckedAt)
	if err != nil {
		t.Fatalf("targets: не удалось распарсить res.CheckedAt: %v", err)
	}
	gotTime, err := time.Parse(timeLayout, gotLastCheck)
	if err != nil {
		t.Fatalf("targets: не удалось распарсить gotLastCheck %q: %v", gotLastCheck, err)
	}
	if !wantTime.Equal(gotTime) {
		t.Fatalf("несовпадение last_check для target_id=%d: ожидали %q, получили %q", id, res.CheckedAt, gotLastCheck)
	}
}
