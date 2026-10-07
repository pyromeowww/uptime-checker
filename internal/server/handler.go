package server

import (
	"encoding/json"
	"log"
	"net/http"
	"sync"

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
	var (
		results []domain.CheckResult
		mu      sync.Mutex
		wg      sync.WaitGroup
	)
	for _, addr := range req.Addresses {
		wg.Add(1)
		go func(url string) {
			defer wg.Done()
			targetID, err := h.store.GetOrCreateTarget(r.Context(), addr)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "Failed to target DB "+err.Error())
				return
			}
			checkResult, err := h.checker.Check(addr)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "Check error: "+err.Error())
				return
			}
			// Берем первый элемент из слайса
			checkRes := checkResult[0]

			// Присваиваем TargetID
			checkRes.TargetID = targetID

			// Сохраняем в базу данных
			if err := h.store.SaveResult(r.Context(), checkRes); err != nil {
				writeError(w, http.StatusInternalServerError, "Failed to save result: "+err.Error())
				return
			}
			mu.Lock()
			// Добавляем в итоговый массив для HTTP-ответа
			results = append(results, checkRes)
			mu.Unlock()
		}(addr)
	}
	wg.Wait()
	res := domain.CheckResponse{
		Results: results,
	}

	writeJSON(w, http.StatusOK, res)
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
