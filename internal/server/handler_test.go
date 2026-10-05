package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pyromeowww/uptime-checker/internal/checker"
	"github.com/pyromeowww/uptime-checker/internal/domain"
)

// Проверяем метод запроса
func TestCheckHandler_MethodNotAllowed(t *testing.T) {
	// Создаем экземпляр хэндлера
	chk := checker.NewChecker()
	h := NewHandler(chk)

	req := httptest.NewRequest(http.MethodGet, "/check", nil)
	recorder := httptest.NewRecorder()
	h.CheckHandler(recorder, req)
	if recorder.Code != http.StatusMethodNotAllowed {
		t.Errorf("ожидали статус %d, получили %d", http.StatusMethodNotAllowed, recorder.Code)
	}
}

// Проверяем передачу кривой строки
func TestCheckHandler_InvalidJSON(t *testing.T) {
	chk := checker.NewChecker()
	h := NewHandler(chk)

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
	h := NewHandler(chk)

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
