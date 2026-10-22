package server

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/pyromeowww/uptime-checker/internal/checker"
	"github.com/pyromeowww/uptime-checker/internal/db"
	"github.com/pyromeowww/uptime-checker/internal/domain"
)

type Handler struct {
	checker *checker.Checker
	store   *db.Store // Сохраняем доступ к БД
}

func NewHandler(chk *checker.Checker, store *db.Store) *Handler {
	return &Handler{
		checker: chk,
		store:   store,
	}
}

func (h *Handler) CheckHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req domain.CheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "JSON deserialization error: "+err.Error())
		return
	}
	if len(req.Addresses) == 0 {
		writeError(w, http.StatusBadRequest, "addresses list is empty")
		return
	}

	ctx := r.Context()

	var (
		results  []domain.CheckResult
		mu       sync.Mutex
		wg       sync.WaitGroup
		hasError bool
		errStr   string
	)

	for _, addr := range req.Addresses {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()

			targetID, err := h.store.GetOrCreateTarget(ctx, url)
			if err != nil {
				mu.Lock()
				hasError = true
				errStr = "Failed to target DB: " + err.Error()
				mu.Unlock()
				return
			}

			checkResult, err := h.checker.Check(url)
			if err != nil {
				mu.Lock()
				hasError = true
				errStr = "Check error: " + err.Error()
				mu.Unlock()
				return
			}
			checkRes := checkResult[0]
			checkRes.TargetID = targetID

			if err := h.store.SaveResult(ctx, checkRes); err != nil {
				mu.Lock()
				hasError = true
				errStr = "Failed to save result: " + err.Error()
				mu.Unlock()
				return
			}

			mu.Lock()
			results = append(results, checkRes)
			mu.Unlock()
		}(addr)
	}
	wg.Wait()

	if hasError {
		writeError(w, http.StatusInternalServerError, errStr)
		return
	}

	writeJSON(w, http.StatusOK, domain.CheckResponse{Results: results})
}

func (h *Handler) GetHistoryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	ctx := r.Context()

	// Считываем общий параметр period (по умолчанию 24h)
	periodStr := r.URL.Query().Get("period")
	if periodStr == "" {
		periodStr = "24h"
	}
	period, err := time.ParseDuration(periodStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid period format: "+err.Error())
		return
	}

	// Клиент передал URL
	targetURL := r.URL.Query().Get("url")
	if targetURL != "" {
		history, err := h.store.GetHistoryByUrl(ctx, targetURL, period)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal server error: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, history)
		return
	}

	// Клиент передал target_id
	targetIDStr := r.URL.Query().Get("target_id")
	if targetIDStr != "" {
		targetID, err := strconv.ParseInt(targetIDStr, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid target_id: "+err.Error())
			return
		}

		history, err := h.store.GetTargetHistory(ctx, targetID, period)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "internal server error: "+err.Error())
			return
		}
		writeJSON(w, http.StatusOK, history)
		return
	}

	// Если не передали ни url, ни target_id
	writeError(w, http.StatusBadRequest, "either 'url' or 'target_id' parameter is required")
}

func (h *Handler) GetAllHistoryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}
	ctx := r.Context()

	periodStr := r.URL.Query().Get("period")
	if periodStr == "" {
		periodStr = "24h" // по умолчанию за последний день
	}
	period, err := time.ParseDuration(periodStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid period format: "+err.Error())
		return
	}

	history, err := h.store.GetAllHistory(ctx, period)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, history)
}

func (h *Handler) CreateTargetHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// 1. Декодируем тело запроса
	var req domain.SubscriptionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "JSON deserialization error: "+err.Error())
		return
	}

	if len(req.URLs) == 0 && len(req.TargetIDs) == 0 {
		writeError(w, http.StatusBadRequest, "at least one URL or target_id must be provided")
		return
	}

	// Определяем период проверки
	periodStr := req.Period
	if periodStr == "" {
		periodStr = "5m" // Дефолтное значение
	}

	period, err := time.ParseDuration(periodStr)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid period format: use format like '1m', '5m', '1h'")
		return
	}

	minPeriod := 1 * time.Minute
	if period < minPeriod {
		writeError(w, http.StatusBadRequest, "interval cannot be less than 1 minute")
		return
	}

	// 2. Валидация каждого URL на корректность
	for _, rawURL := range req.URLs {
		parsedURL, err := url.ParseRequestURI(rawURL)
		if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
			writeError(w, http.StatusBadRequest, "invalid URL format: "+rawURL)
			return
		}
	}

	// 3. Валидация каждого ID (должен быть больше 0)
	for _, id := range req.TargetIDs {
		if id <= 0 {
			writeError(w, http.StatusBadRequest, "invalid target_id: must be greater than zero")
			return
		}
	}

	// 4. Передаем контекст r.Context() и запрос в хранилище
	res, err := h.store.CreateSubscriptions(r.Context(), req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to create subscription: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, res)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("encode response: %v", err)
	}
}

func writeError(w http.ResponseWriter, status int, errText string) {
	writeJSON(w, status, map[string]string{"error": errText})
}
