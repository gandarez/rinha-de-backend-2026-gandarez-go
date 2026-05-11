package fraud

import (
	"os"
	"testing"
)

func TestLoadReferencesFromReader(t *testing.T) {
	f, err := os.Open("../../resources/example-references.json")
	if err != nil {
		t.Fatalf("open example-references.json: %v", err)
	}
	defer f.Close()

	idx, err := loadReferencesFromReader(f, 0)
	if err != nil {
		t.Fatalf("loadReferencesFromReader: %v", err)
	}
	if idx.Len() == 0 {
		t.Fatal("expected at least one reference vector, got 0")
	}

	// Verify sentinel values are preserved: look for a vector with -1 at index 5
	sentinelFound := false
	for i := 0; i < idx.n; i++ {
		off := i * VectorDim
		if idx.vectors[off+5] == -1 {
			sentinelFound = true
			if idx.vectors[off+6] != -1 {
				t.Errorf("vector %d has -1 at dim 5 but not at dim 6", i)
			}
			break
		}
	}
	if !sentinelFound {
		t.Log("no sentinel -1 found in example dataset; all references may have prior transactions")
	}

	t.Logf("loaded %d reference vectors", idx.Len())
}
