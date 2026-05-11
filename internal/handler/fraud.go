package handler

import (
	"encoding/json"
	"net/http"

	"github.com/gandarez/rinha-de-backend-2026-gandarez-go/internal/fraud"
	"github.com/gandarez/rinha-de-backend-2026-gandarez-go/internal/model"
)

type FraudHandler struct {
	scorer *fraud.Scorer
}

func New(s *fraud.Scorer) *FraudHandler {
	return &FraudHandler{scorer: s}
}

func (h *FraudHandler) Score(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req model.Request
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	resp := h.scorer.Score(&req)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		return
	}
}
