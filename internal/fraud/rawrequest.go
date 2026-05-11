package fraud

import "time"

// RawRequest holds exactly the fields needed by VectorizeRaw, populated
// by the hand-rolled JSON parser to avoid encoding/json reflection overhead.
type RawRequest struct {
	TxAmount        float64
	TxInstallments  int
	TxTime          time.Time
	CustAvgAmount   float64
	CustTxCount24h  int
	UnknownMerchant bool   // !known: merchant not in customer's known list
	MCC             string // 4-char MCC code
	MerchantAvg     float64
	TermIsOnline    bool
	TermCardPresent bool
	TermKmFromHome  float64
	HasLastTx       bool
	LastTxTime      time.Time
	LastKmFromCurrent float64
}
