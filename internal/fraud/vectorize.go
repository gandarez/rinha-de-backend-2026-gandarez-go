package fraud

import (
	"github.com/gandarez/rinha-de-backend-2026-gandarez-go/internal/model"
)

// Constants mirrors normalization.json — denominators used in the 14-dim vectorization.
type Constants struct {
	MaxAmount            float32 `json:"max_amount"`
	MaxInstallments      float32 `json:"max_installments"`
	AmountVsAvgRatio     float32 `json:"amount_vs_avg_ratio"`
	MaxMinutes           float32 `json:"max_minutes"`
	MaxKm                float32 `json:"max_km"`
	MaxTxCount24h        float32 `json:"max_tx_count_24h"`
	MaxMerchantAvgAmount float32 `json:"max_merchant_avg_amount"`
}

// Vectorizer converts a Request into the 14-dimensional float32 vector used for KNN.
type Vectorizer struct {
	c   Constants
	mcc map[string]float32
}

// NewVectorizer constructs a Vectorizer with normalization constants and an MCC risk map.
func NewVectorizer(c Constants, mcc map[string]float32) *Vectorizer {
	return &Vectorizer{c: c, mcc: mcc}
}

func clamp(x float32) float32 {
	if x < 0 {
		return 0
	}
	if x > 1 {
		return 1
	}
	return x
}

// Vectorize builds the 14-dim vector for req following REGRAS_DE_DETECCAO.md.
// Indices 5 and 6 are -1 when LastTransaction is nil (sentinel for "no prior tx").
func (v *Vectorizer) Vectorize(req *model.Request) [VectorDim]float32 {
	var vec [VectorDim]float32

	// 0: amount
	vec[0] = clamp(float32(req.Transaction.Amount) / v.c.MaxAmount)

	// 1: installments
	vec[1] = clamp(float32(req.Transaction.Installments) / v.c.MaxInstallments)

	// 2: amount_vs_avg — guard zero average
	if req.Customer.AvgAmount > 0 {
		vec[2] = clamp((float32(req.Transaction.Amount) / float32(req.Customer.AvgAmount)) / v.c.AmountVsAvgRatio)
	}

	t := req.Transaction.RequestedAt.UTC()

	// 3: hour_of_day (0-23)
	vec[3] = float32(t.Hour()) / 23.0

	// 4: day_of_week — Go: Sun=0..Sat=6; spec: Mon=0..Sun=6
	vec[4] = float32((int(t.Weekday())+6)%7) / 6.0

	// 5, 6: minutes_since_last_tx and km_from_last_tx — sentinel -1 when no prior tx
	if req.LastTransaction == nil {
		vec[5] = -1
		vec[6] = -1
	} else {
		minutes := req.Transaction.RequestedAt.Sub(req.LastTransaction.Timestamp).Minutes()
		vec[5] = clamp(float32(minutes) / v.c.MaxMinutes)
		vec[6] = clamp(float32(req.LastTransaction.KmFromCurrent) / v.c.MaxKm)
	}

	// 7: km_from_home
	vec[7] = clamp(float32(req.Terminal.KmFromHome) / v.c.MaxKm)

	// 8: tx_count_24h
	vec[8] = clamp(float32(req.Customer.TxCount24h) / v.c.MaxTxCount24h)

	// 9: is_online
	if req.Terminal.IsOnline {
		vec[9] = 1
	}

	// 10: card_present
	if req.Terminal.CardPresent {
		vec[10] = 1
	}

	// 11: unknown_merchant — 1 if merchant NOT in known list
	known := false
	for _, m := range req.Customer.KnownMerchants {
		if m == req.Merchant.ID {
			known = true
			break
		}
	}
	if !known {
		vec[11] = 1
	}

	// 12: mcc_risk — default 0.5 for unknown MCCs
	if risk, ok := v.mcc[req.Merchant.MCC]; ok {
		vec[12] = risk
	} else {
		vec[12] = 0.5
	}

	// 13: merchant_avg_amount
	vec[13] = clamp(float32(req.Merchant.AvgAmount) / v.c.MaxMerchantAvgAmount)

	return vec
}

// VectorizeRaw builds the 14-dim vector from a RawRequest (pre-parsed by the fast handler).
func (v *Vectorizer) VectorizeRaw(req *RawRequest) [VectorDim]float32 {
	var vec [VectorDim]float32

	// 0: amount
	vec[0] = clamp(float32(req.TxAmount) / v.c.MaxAmount)

	// 1: installments
	vec[1] = clamp(float32(req.TxInstallments) / v.c.MaxInstallments)

	// 2: amount_vs_avg
	if req.CustAvgAmount > 0 {
		vec[2] = clamp((float32(req.TxAmount) / float32(req.CustAvgAmount)) / v.c.AmountVsAvgRatio)
	}

	// 3: hour_of_day
	vec[3] = float32(req.TxTime.Hour()) / 23.0

	// 4: day_of_week (Mon=0..Sun=6)
	vec[4] = float32((int(req.TxTime.Weekday())+6)%7) / 6.0

	// 5, 6: minutes_since_last_tx and km_from_last_tx — sentinel -1 when no prior tx
	if !req.HasLastTx {
		vec[5] = -1
		vec[6] = -1
	} else {
		minutes := req.TxTime.Sub(req.LastTxTime).Minutes()
		vec[5] = clamp(float32(minutes) / v.c.MaxMinutes)
		vec[6] = clamp(float32(req.LastKmFromCurrent) / v.c.MaxKm)
	}

	// 7: km_from_home
	vec[7] = clamp(float32(req.TermKmFromHome) / v.c.MaxKm)

	// 8: tx_count_24h
	vec[8] = clamp(float32(req.CustTxCount24h) / v.c.MaxTxCount24h)

	// 9: is_online
	if req.TermIsOnline {
		vec[9] = 1
	}

	// 10: card_present
	if req.TermCardPresent {
		vec[10] = 1
	}

	// 11: unknown_merchant
	if req.UnknownMerchant {
		vec[11] = 1
	}

	// 12: mcc_risk
	if risk, ok := v.mcc[req.MCC]; ok {
		vec[12] = risk
	} else {
		vec[12] = 0.5
	}

	// 13: merchant_avg_amount
	vec[13] = clamp(float32(req.MerchantAvg) / v.c.MaxMerchantAvgAmount)

	return vec
}
