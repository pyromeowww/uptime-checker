package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pyromeowww/uptime-checker/internal/checker"
	"github.com/pyromeowww/uptime-checker/internal/config"
	"github.com/pyromeowww/uptime-checker/internal/db"
	"github.com/pyromeowww/uptime-checker/internal/domain"
)

// Проверяем метод запроса
func TestHandlers_MethodNotAllowed(t *testing.T) {
	chk := checker.NewChecker()
	store := newTestStore(t)
	h := NewHandler(chk, store)

	tests := []struct {
		name    string
		method  string
		path    string
		handler http.HandlerFunc
	}{
		{
			name:    "CheckHandler expects POST",
			method:  http.MethodGet,
			path:    "/check",
			handler: h.CheckHandler,
		},
		{
			name:    "GetHistoryHandler expects GET",
			method:  http.MethodPost,
			path:    "/history",
			handler: h.GetHistoryHandler,
		},
		{
			name:    "GetAllHistoryHandler expects GET",
			method:  http.MethodPost,
			path:    "/history/all",
			handler: h.GetAllHistoryHandler,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			recorder := httptest.NewRecorder()

			tt.handler(recorder, req)

			if recorder.Code != http.StatusMethodNotAllowed {
				t.Errorf("для %s ожидадали статус %d, получили %d", tt.path, http.StatusMethodNotAllowed, recorder.Code)
			}
		})
	}
}

// Проверяем передачу кривой строки
func TestCheckHandler_InvalidJSON(t *testing.T) {
	chk := checker.NewChecker()
	store := newTestStore(t)
	h := NewHandler(chk, store)

	body := bytes.NewBufferString(`{invalid json}`)
	req := httptest.NewRequest(http.MethodPost, "/check", body)
	recorder := httptest.NewRecorder()
	h.CheckHandler(recorder, req)
	if recorder.Code != http.StatusBadRequest {
		t.Errorf("ожидали статус %d, получили %d", http.StatusBadRequest, recorder.Code)
	}
}

func TestCheckHandler_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	chk := checker.NewChecker()
	store := newTestStore(t)
	h := NewHandler(chk, store)

	reqBody := `{"addresses": ["` + server.URL + `"]}`
	req := httptest.NewRequest(http.MethodPost, "/check", bytes.NewBufferString(reqBody))
	recorder := httptest.NewRecorder()
	h.CheckHandler(recorder, req)
	var resp domain.CheckResponse
	if err := json.NewDecoder(recorder.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if recorder.Code != http.StatusOK {
		t.Errorf("ожидали статус %d, получили %d", http.StatusOK, recorder.Code)
	}

	if len(resp.Results) != 1 {
		t.Errorf("ожидали длинну результата %d, получили %d", 1, len(resp.Results))
	}

	if resp.Results[0].Status != domain.StatusUp {
		t.Errorf("ожидали статус %s, получили %s", domain.StatusUp, resp.Results[0].Status)
	}
}

func TestHistoryHandlers_Scenarios(t *testing.T) {
	chk := checker.NewChecker()
	store := newTestStore(t)
	h := NewHandler(chk, store)

	tests := []struct {
		name           string
		path           string
		handler        http.HandlerFunc
		expectedStatus int
	}{
		{
			name:           "GetAllHistory: success",
			path:           "/history/all",
			handler:        h.GetAllHistoryHandler,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "GetHistory: success with query param",
			path:           "/history?url=https://github.com",
			handler:        h.GetHistoryHandler,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "GetHistory: missing url or id parameter",
			path:           "/history",
			handler:        h.GetHistoryHandler,
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			recorder := httptest.NewRecorder()
			tt.handler(recorder, req)
			if recorder.Code != tt.expectedStatus {
				t.Errorf("ожидали статус %d, получили %d", tt.expectedStatus, recorder.Code)
			}
		})
	}
}

func TestCheckHandler_DownDoesNotFailRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	chk := checker.NewChecker()
	store := newTestStore(t)
	h := NewHandler(chk, store)

	body := `{"addresses":["` + server.URL + `","http://127.0.0.1:1"]}`
	req := httptest.NewRequest(http.MethodPost, "/check", strings.NewReader(body))
	rec := httptest.NewRecorder()

	h.CheckHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("DOWN не должен ронять запрос: ожидали 200, получили %d: %s", rec.Code, rec.Body.String())
	}

	var resp domain.CheckResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("ожидали 2 результата, получили %d", len(resp.Results))
	}
	if resp.Results[0].Status != domain.StatusUp {
		t.Errorf("results[0] должен быть UP, получили %s", resp.Results[0].Status)
	}
	if resp.Results[1].Status != domain.StatusDown {
		t.Errorf("results[1] должен быть DOWN, получили %s", resp.Results[1].Status)
	}
}

func TestCheckHandler_AllFailed(t *testing.T) {
	store := newTestStore(t)
	// закрываем БД: любой GetOrCreateTarget вернёт ошибку
	if err := store.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	h := NewHandler(checker.NewChecker(), store)
	body := `{"addresses":["http://a.example","http://b.example"]}`
	req := httptest.NewRequest(http.MethodPost, "/check", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.CheckHandler(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("ожидали 500, получили %d", rec.Code)
	}
	var errResp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if errResp["error"] != "all checks failed" {
		t.Errorf("ожидали 'all checks failed', получили %q", errResp["error"])
	}
}

func TestDeleteSubscriptionsHandler(t *testing.T) {
	store := newTestStore(t)
	h := NewHandler(checker.NewChecker(), store)

	if _, err := store.CreateSubscriptions(context.Background(), domain.SubscriptionRequest{
		URLs:        []string{"https://a.example", "https://b.example", "https://c.example"},
		ContactInfo: "tg:@user",
		Period:      "5m",
	}); err != nil {
		t.Fatalf("подготовка: %v", err)
	}

	body := `{"url":["https://a.example","https://c.example"]}`
	req := httptest.NewRequest(http.MethodDelete, "/api/subscriptions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.DeleteSubscriptionsHandler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("ожидали 200, получили %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]int64
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["deleted"] != 2 {
		t.Errorf("ожидали deleted=2, получили %d", resp["deleted"])
	}
}

func newTestStore(t *testing.T) *db.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	cfg := config.Config{
		DBFile: path,
	}
	store, err := db.New(&cfg)
	if err != nil {
		t.Fatalf("не удалось создать тестовый store: %v", err)
	}

	t.Cleanup(func() {
		_ = store.Close() // явно игнорируем повторное закрытие
	})
	return store
}
