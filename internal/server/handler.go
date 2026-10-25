package server

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"sync/atomic"
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

const (
	checkTimeLayout = "2006-01-02 15:04:05"
	defaultLimit    = 100
	maxLimit        = 1000
)

func (h *Handler) CheckHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
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

	results := make([]domain.CheckResult, len(req.Addresses))

	var (
		wg     sync.WaitGroup
		failed atomic.Int32
	)

	for i, addr := range req.Addresses {
		wg.Add(1)
		go func(idx int, rawURL string) {
			defer wg.Done()

			// Нормализуем один раз, чтобы в targets не плодились дубликаты
			u := checker.NormalizeURL(rawURL)
			now := time.Now().Format(checkTimeLayout)

			targetID, err := h.store.GetOrCreateTarget(ctx, u)
			if err != nil {
				failed.Add(1)
				results[idx] = domain.CheckResult{
					Status:    domain.StatusDown,
					ErrMsg:    "failed to get target from DB: " + err.Error(),
					CheckedAt: now,
				}
				return
			}

			checkResult, err := h.checker.Check(u)
			if err != nil {
				failed.Add(1)
				results[idx] = domain.CheckResult{
					TargetID:  targetID,
					Status:    domain.StatusDown,
					ErrMsg:    "check error: " + err.Error(),
					CheckedAt: now,
				}
				return
			}
			if len(checkResult) == 0 {
				failed.Add(1)
				results[idx] = domain.CheckResult{
					TargetID:  targetID,
					Status:    domain.StatusDown,
					ErrMsg:    "check returned no result",
					CheckedAt: now,
				}
				return
			}

			checkRes := checkResult[0]
			checkRes.TargetID = targetID

			if err := h.store.SaveResult(ctx, checkRes); err != nil {
				checkRes.ErrMsg = "failed to save result: " + err.Error()
			}

			results[idx] = checkRes
		}(i, addr)
	}
	wg.Wait()

	// Полный провал: ни одна проверка не удалась.
	if failed.Load() == int32(len(req.Addresses)) {
		writeError(w, http.StatusInternalServerError, "all checks failed")
		return
	}

	writeJSON(w, http.StatusOK, domain.CheckResponse{Results: results})
}

func (h *Handler) GetHistoryHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
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
		targetURL = checker.NormalizeURL(targetURL)
		history, err := h.store.GetHistoryByURL(ctx, targetURL, period)
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
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
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
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
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
		periodStr = "1m" // Дефолтное значение
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
	for i, rawURL := range req.URLs {
		parsedURL, err := url.ParseRequestURI(rawURL)
		if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
			writeError(w, http.StatusBadRequest, "invalid URL format: "+rawURL)
			return
		}
		req.URLs[i] = checker.NormalizeURL(rawURL)
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
		writeError(w, http.StatusInternalServerError, "failed to create subscription: "+err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, res)
}

func (h *Handler) GetAllSubscriptionsHandler(w http.ResponseWriter, r *http.Request) {
	limit, offset, err := parsePagination(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	subs, err := h.store.GetAllSubscriptions(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get subscriptions: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, domain.SubscriptionListResponse{Subscriptions: subs})
}

func (h *Handler) DeleteSubscriptionsHandler(w http.ResponseWriter, r *http.Request) {
	var req struct {
		URLs []string `json:"url"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "JSON deserialization error: "+err.Error())
		return
	}
	if len(req.URLs) == 0 {
		writeError(w, http.StatusBadRequest, "urls list is empty")
		return
	}

	urls := make([]string, len(req.URLs))
	for i, u := range req.URLs {
		if u == "" {
			writeError(w, http.StatusBadRequest, "url at index "+strconv.Itoa(i)+" is empty")
			return
		}
		urls[i] = checker.NormalizeURL(u)
	}

	count, err := h.store.DeleteSubscriptionsByURLs(r.Context(), urls)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete subscriptions: "+err.Error())
		return
	}
	if count == 0 {
		writeError(w, http.StatusNotFound, "no subscriptions found for the given urls")
		return
	}

	writeJSON(w, http.StatusOK, map[string]int64{"deleted": count})
}

func (h *Handler) DeleteAllSubscriptionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}
	ctx := r.Context()
	count, err := h.store.DeleteAllSubscriptions(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete all subscriptions: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]int64{"deleted": count})
}

// parsePagination парсит ?limit=&offset=. Возвращает (limit, offset, error).
func parsePagination(r *http.Request) (int, int, error) {
	limit := defaultLimit
	offset := 0

	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return 0, 0, fmt.Errorf("invalid limit: must be a positive integer")
		}
		if n > maxLimit {
			n = maxLimit
		}
		limit = n
	}

	if v := r.URL.Query().Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return 0, 0, fmt.Errorf("invalid offset: must be a non-negative integer")
		}
		offset = n
	}

	return limit, offset, nil
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
