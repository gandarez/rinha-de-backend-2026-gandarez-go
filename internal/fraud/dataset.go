package fraud

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// LoadConstants reads normalization.json from path.
func LoadConstants(path string) (Constants, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Constants{}, fmt.Errorf("read constants: %w", err)
	}
	var c Constants
	if err := json.Unmarshal(data, &c); err != nil {
		return Constants{}, fmt.Errorf("parse constants: %w", err)
	}
	return c, nil
}

// LoadMCCRisk reads mcc_risk.json from path. Missing MCCs default to 0.5 at query time.
func LoadMCCRisk(path string) (map[string]float32, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read mcc_risk: %w", err)
	}
	m := make(map[string]float32)
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("parse mcc_risk: %w", err)
	}
	return m, nil
}

// LoadReferences decompresses and streams references.json.gz from path into an Index.
func LoadReferences(path string) (*Index, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open references: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	return loadReferencesFromReader(gz, 3_000_000)
}

// loadReferencesFromReader streams JSON array records from r into an Index.
// capacity hints the initial allocation (0 = grow dynamically).
func loadReferencesFromReader(r io.Reader, capacity int) (*Index, error) {
	dec := json.NewDecoder(r)

	tok, err := dec.Token()
	if err != nil {
		return nil, fmt.Errorf("opening token: %w", err)
	}
	if d, ok := tok.(json.Delim); !ok || d != '[' {
		return nil, fmt.Errorf("expected '[', got %v", tok)
	}

	type refRecord struct {
		Vector [VectorDim]float32 `json:"vector"`
		Label  string             `json:"label"`
	}

	idx := NewIndex(capacity)
	for dec.More() {
		var rec refRecord
		if err := dec.Decode(&rec); err != nil {
			return nil, fmt.Errorf("decode record: %w", err)
		}
		idx.Add(rec.Vector, rec.Label == "fraud")
	}

	return idx, nil
}
