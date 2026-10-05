package server

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/pyromeowww/uptime-checker/internal/checker"
	"github.com/pyromeowww/uptime-checker/internal/domain"
)

type Handler struct {
	checker *checker.Checker
}

func NewHandler(chk *checker.Checker) *Handler {
	return &Handler{
		checker: chk,
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

	var res domain.CheckResponse
	for _, addr := range req.Addresses {
		checkAddr, _ := h.checker.Check(addr)
		res.Results = append(res.Results, checkAddr)
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
