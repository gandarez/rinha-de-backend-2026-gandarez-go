package fraud

import (
	"testing"
	"time"

	"github.com/gandarez/rinha-de-backend-2026-gandarez-go/internal/model"
)

func TestVectorize(t *testing.T) {
	consts := Constants{
		MaxAmount:            10000,
		MaxInstallments:      12,
		AmountVsAvgRatio:     10,
		MaxMinutes:           1440,
		MaxKm:                1000,
		MaxTxCount24h:        20,
		MaxMerchantAvgAmount: 10000,
	}
	mccRisk := map[string]float32{
		"5411": 0.15,
		"7802": 0.75,
	}
	v := NewVectorizer(consts, mccRisk)

	tests := []struct {
		name string
		req  model.Request
		want [VectorDim]float32
	}{
		{
			// Legit example from REGRAS_DE_DETECCAO.md
			name: "legit tx-1329056812",
			req: model.Request{
				ID: "tx-1329056812",
				Transaction: model.Transaction{
					Amount:       41.12,
					Installments: 2,
					RequestedAt:  time.Date(2026, 3, 11, 18, 45, 53, 0, time.UTC),
				},
				Customer: model.Customer{
					AvgAmount:      82.24,
					TxCount24h:     3,
					KnownMerchants: []string{"MERC-003", "MERC-016"},
				},
				Merchant: model.Merchant{
					ID:        "MERC-016",
					MCC:       "5411",
					AvgAmount: 60.25,
				},
				Terminal: model.Terminal{
					IsOnline:    false,
					CardPresent: true,
					KmFromHome:  29.23,
				},
				LastTransaction: nil,
			},
			want: [VectorDim]float32{0.0041, 0.1667, 0.05, 0.7826, 0.3333, -1, -1, 0.0292, 0.15, 0, 1, 0, 0.15, 0.006},
		},
		{
			// Fraud example from REGRAS_DE_DETECCAO.md
			name: "fraud tx-3330991687",
			req: model.Request{
				ID: "tx-3330991687",
				Transaction: model.Transaction{
					Amount:       9505.97,
					Installments: 10,
					RequestedAt:  time.Date(2026, 3, 14, 5, 15, 12, 0, time.UTC),
				},
				Customer: model.Customer{
					AvgAmount:      81.28,
					TxCount24h:     20,
					KnownMerchants: []string{"MERC-008", "MERC-007", "MERC-005"},
				},
				Merchant: model.Merchant{
					ID:        "MERC-068",
					MCC:       "7802",
					AvgAmount: 54.86,
				},
				Terminal: model.Terminal{
					IsOnline:    false,
					CardPresent: true,
					KmFromHome:  952.27,
				},
				LastTransaction: nil,
			},
			want: [VectorDim]float32{0.9506, 0.8333, 1.0, 0.2174, 0.8333, -1, -1, 0.9523, 1.0, 0, 1, 1, 0.75, 0.0055},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := v.Vectorize(&tc.req)
			for i := range got {
				wantVal := tc.want[i]
				gotVal := got[i]
				// Sentinel -1 must match exactly
				if wantVal == -1 || gotVal == -1 {
					if gotVal != wantVal {
						t.Errorf("dim[%d]: got %v, want %v", i, gotVal, wantVal)
					}
					continue
				}
				diff := gotVal - wantVal
				if diff < 0 {
					diff = -diff
				}
				if diff > 1e-4 {
					t.Errorf("dim[%d]: got %.4f, want %.4f (diff %.6f)", i, gotVal, wantVal, diff)
				}
			}
		})
	}
}
