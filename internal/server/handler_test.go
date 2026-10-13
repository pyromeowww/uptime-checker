package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
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
	json.NewDecoder(recorder.Body).Decode(&resp)

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
		store.Close()
	})
	return store
}
