package handler

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"github.com/gandarez/rinha-de-backend-2026-gandarez-go/internal/fraud"
	"github.com/gandarez/rinha-de-backend-2026-gandarez-go/internal/model"
)

var sampleBodies = []string{
	`{"id":"tx-1","transaction":{"amount":2508.13,"installments":7,"requested_at":"2026-03-11T03:45:53Z"},"customer":{"avg_amount":209.74,"tx_count_24h":13,"known_merchants":["MERC-003","MERC-016"]},"merchant":{"id":"MERC-089","mcc":"7801","avg_amount":25.15},"terminal":{"is_online":false,"card_present":true,"km_from_home":667.7296579973},"last_transaction":null}`,
	`{"id":"tx-2","transaction":{"amount":384.88,"installments":3,"requested_at":"2026-03-11T20:23:35Z"},"customer":{"avg_amount":769.76,"tx_count_24h":3,"known_merchants":["MERC-009","MERC-001"]},"merchant":{"id":"MERC-001","mcc":"5912","avg_amount":298.95},"terminal":{"is_online":false,"card_present":true,"km_from_home":13.7090520965},"last_transaction":{"timestamp":"2026-03-11T14:58:35Z","km_from_current":18.8626479774}}`,
	`{"id":"tx-3","transaction":{"amount":100.0,"installments":1,"requested_at":"2026-01-15T08:00:00Z"},"customer":{"avg_amount":0,"tx_count_24h":0,"known_merchants":[]},"merchant":{"id":"MERC-XYZ","mcc":"5411","avg_amount":50.0},"terminal":{"is_online":true,"card_present":false,"km_from_home":0.0},"last_transaction":null}`,
}

func TestParseBodyMatchesEncodingJSON(t *testing.T) {
	for _, body := range sampleBodies {
		var ref model.Request
		if err := json.Unmarshal([]byte(body), &ref); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}

		var raw fraud.RawRequest
		if err := parseBody([]byte(body), &raw); err != nil {
			t.Fatalf("parseBody: %v", err)
		}

		if !approxEq(raw.TxAmount, ref.Transaction.Amount, 1e-6) {
			t.Errorf("TxAmount: got %v want %v", raw.TxAmount, ref.Transaction.Amount)
		}
		if raw.TxInstallments != ref.Transaction.Installments {
			t.Errorf("TxInstallments: got %v want %v", raw.TxInstallments, ref.Transaction.Installments)
		}
		if raw.TxTime.Hour() != ref.Transaction.RequestedAt.UTC().Hour() {
			t.Errorf("TxTime.Hour: got %v want %v", raw.TxTime.Hour(), ref.Transaction.RequestedAt.UTC().Hour())
		}
		if raw.TxTime.Weekday() != ref.Transaction.RequestedAt.UTC().Weekday() {
			t.Errorf("TxTime.Weekday: got %v want %v", raw.TxTime.Weekday(), ref.Transaction.RequestedAt.UTC().Weekday())
		}
		if !approxEq(raw.CustAvgAmount, ref.Customer.AvgAmount, 1e-6) {
			t.Errorf("CustAvgAmount: got %v want %v", raw.CustAvgAmount, ref.Customer.AvgAmount)
		}
		if raw.CustTxCount24h != ref.Customer.TxCount24h {
			t.Errorf("CustTxCount24h: got %v want %v", raw.CustTxCount24h, ref.Customer.TxCount24h)
		}
		known := false
		for _, m := range ref.Customer.KnownMerchants {
			if m == ref.Merchant.ID {
				known = true
				break
			}
		}
		if raw.UnknownMerchant != !known {
			t.Errorf("UnknownMerchant: got %v want %v", raw.UnknownMerchant, !known)
		}
		if raw.MCC != ref.Merchant.MCC {
			t.Errorf("MCC: got %q want %q", raw.MCC, ref.Merchant.MCC)
		}
		if !approxEq(raw.MerchantAvg, ref.Merchant.AvgAmount, 1e-6) {
			t.Errorf("MerchantAvg: got %v want %v", raw.MerchantAvg, ref.Merchant.AvgAmount)
		}
		if raw.TermIsOnline != ref.Terminal.IsOnline {
			t.Errorf("TermIsOnline: got %v want %v", raw.TermIsOnline, ref.Terminal.IsOnline)
		}
		if raw.TermCardPresent != ref.Terminal.CardPresent {
			t.Errorf("TermCardPresent: got %v want %v", raw.TermCardPresent, ref.Terminal.CardPresent)
		}
		if !approxEq(raw.TermKmFromHome, ref.Terminal.KmFromHome, 1e-6) {
			t.Errorf("TermKmFromHome: got %v want %v", raw.TermKmFromHome, ref.Terminal.KmFromHome)
		}
		hasLast := ref.LastTransaction != nil
		if raw.HasLastTx != hasLast {
			t.Errorf("HasLastTx: got %v want %v", raw.HasLastTx, hasLast)
		}
		if hasLast {
			if !approxEq(raw.LastKmFromCurrent, ref.LastTransaction.KmFromCurrent, 1e-6) {
				t.Errorf("LastKmFromCurrent: got %v want %v", raw.LastKmFromCurrent, ref.LastTransaction.KmFromCurrent)
			}
			wantTs := ref.LastTransaction.Timestamp.UTC().Truncate(time.Second)
			gotTs := raw.LastTxTime.UTC().Truncate(time.Second)
			if !gotTs.Equal(wantTs) {
				t.Errorf("LastTxTime: got %v want %v", gotTs, wantTs)
			}
		}
	}
}

func approxEq(a, b, eps float64) bool {
	if a == b {
		return true
	}
	return math.Abs(a-b) <= eps*math.Max(1, math.Max(math.Abs(a), math.Abs(b)))
}
