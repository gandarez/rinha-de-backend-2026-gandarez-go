package fraud

import "github.com/gandarez/rinha-de-backend-2026-gandarez-go/internal/model"

// Indexer is implemented by both *Index (brute-force) and *IVFIndex (approximate).
type Indexer interface {
	Score(query [VectorDim]float32) int
}

// Scorer wraps a Vectorizer and an Indexer to produce fraud decisions.
type Scorer struct {
	vectorizer *Vectorizer
	index      Indexer
}

// New creates a Scorer backed by the given vectorizer and index implementation.
func New(v *Vectorizer, idx Indexer) *Scorer {
	return &Scorer{vectorizer: v, index: idx}
}

// Score returns the number of fraud neighbors (0..TopK). The handler maps
// this directly to one of the pre-encoded response byte slices.
func (s *Scorer) Score(req *model.Request) int {
	vec := s.vectorizer.Vectorize(req)
	return s.index.Score(vec)
}

// ScoreRaw is the fast path used by the hand-rolled parser, avoiding model.Request.
func (s *Scorer) ScoreRaw(req *RawRequest) int {
	vec := s.vectorizer.VectorizeRaw(req)
	return s.index.Score(vec)
}
