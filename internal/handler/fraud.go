package handler

import (
	"io"
	"net/http"

	"github.com/gandarez/rinha-de-backend-2026-gandarez-go/internal/fraud"
)

type FraudHandler struct {
	scorer *fraud.Scorer
}

func New(s *fraud.Scorer) *FraudHandler {
	return &FraudHandler{scorer: s}
}

func (h *FraudHandler) Score(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	ws := wsPool.Get().(*workspace)
	defer wsPool.Put(ws)

	// Read body into stack-like buffer from pool (no heap allocation).
	limited := io.LimitReader(r.Body, int64(len(ws.body)))
	n, err := io.ReadFull(limited, ws.body[:])
	if err != nil && err != io.ErrUnexpectedEOF {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}

	if err := parseBody(ws.body[:n], &ws.raw); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	fraudCount := h.scorer.ScoreRaw(&ws.raw)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(responses[fraudCount]) //nolint:errcheck
}
