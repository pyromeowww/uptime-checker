package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
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
	var req domain.CheckRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "JSON deserialization error: "+err.Error())
		return
	}

	if len(req.Addresses) == 0 {
		writeError(w, http.StatusBadRequest, "addresses list is empty")
		return
	}

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

			// Используем Background, так как горутина асинхронная
			ctx := context.Background()

			targetID, err := h.store.GetOrCreateTarget(ctx, url)
			if err != nil {
				mu.Lock()
				hasError = true
				errStr = "Failed to target DB: " + err.Error()
				mu.Unlock()
				return
			}
			log.Printf("URL: %s -> Assigned targetID: %d", url, targetID)

			checkResult, err := h.checker.Check(url)
			if err != nil {
				mu.Lock()
				hasError = true
				errStr = "Check error: " + err.Error()
				mu.Unlock()
				return
			}

			if len(checkResult) == 0 {
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

	// Если во время проверки в горутинах произошла ошибка, возвращаем 500
	if hasError {
		writeError(w, http.StatusInternalServerError, errStr)
		return
	}

	res := domain.CheckResponse{
		Results: results,
	}

	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) GetHistoryHandler(w http.ResponseWriter, r *http.Request) {
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

	// Клиент передал URL (предпочтительный способ)
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

	// Клиент передал target_id (обратная совместимость)
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
