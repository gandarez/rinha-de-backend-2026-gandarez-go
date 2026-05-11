package fraud

import "github.com/gandarez/rinha-de-backend-2026-gandarez-go/internal/model"

// Scorer wraps a Vectorizer and an Index to produce fraud decisions.
type Scorer struct {
	vectorizer *Vectorizer
	index      *Index
}

// New creates a Scorer backed by the given vectorizer and reference index.
func New(v *Vectorizer, idx *Index) *Scorer {
	return &Scorer{vectorizer: v, index: idx}
}

func (s *Scorer) Score(req *model.Request) model.Response {
	vec := s.vectorizer.Vectorize(req)
	score := s.index.Score(vec)
	return model.Response{
		Approved:   score < FraudThreshold,
		FraudScore: float64(score),
	}
}
