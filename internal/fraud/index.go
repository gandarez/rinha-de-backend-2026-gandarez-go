package fraud

import (
	"runtime"
	"sync"
)

const (
	VectorDim      = 14
	TopK           = 5
	FraudThreshold = 0.6
)

// Index holds the flat reference dataset for brute-force KNN search.
// vectors is a flat []float32 of length n*VectorDim; labels is 1=fraud, 0=legit.
type Index struct {
	vectors []float32
	labels  []uint8
	n       int
}

// NewIndex pre-allocates the backing slices for capacity reference vectors.
func NewIndex(capacity int) *Index {
	return &Index{
		vectors: make([]float32, 0, capacity*VectorDim),
		labels:  make([]uint8, 0, capacity),
	}
}

func (idx *Index) Add(v [VectorDim]float32, fraud bool) {
	idx.vectors = append(idx.vectors, v[:]...)
	if fraud {
		idx.labels = append(idx.labels, 1)
	} else {
		idx.labels = append(idx.labels, 0)
	}
	idx.n++
}

func (idx *Index) Len() int { return idx.n }

// topKResult tracks the K closest candidates seen so far.
type topKResult struct {
	dists  [TopK]float32
	labels [TopK]uint8
	size   int
	maxIdx int
}

func (r *topKResult) insert(d float32, label uint8) {
	if r.size < TopK {
		r.dists[r.size] = d
		r.labels[r.size] = label
		r.size++
		if r.size == TopK {
			// Find initial worst slot
			for j := 1; j < TopK; j++ {
				if r.dists[j] > r.dists[r.maxIdx] {
					r.maxIdx = j
				}
			}
		}
	} else if d < r.dists[r.maxIdx] {
		r.dists[r.maxIdx] = d
		r.labels[r.maxIdx] = label
		r.maxIdx = 0
		for j := 1; j < TopK; j++ {
			if r.dists[j] > r.dists[r.maxIdx] {
				r.maxIdx = j
			}
		}
	}
}

// sqDist14 computes the squared Euclidean distance between query q and reference r.
// Fully unrolled for the compiler; BCE hint via _ = r[13].
func sqDist14(q *[VectorDim]float32, r []float32) float32 {
	_ = r[13]
	d0 := q[0] - r[0]
	d1 := q[1] - r[1]
	d2 := q[2] - r[2]
	d3 := q[3] - r[3]
	d4 := q[4] - r[4]
	d5 := q[5] - r[5]
	d6 := q[6] - r[6]
	d7 := q[7] - r[7]
	d8 := q[8] - r[8]
	d9 := q[9] - r[9]
	d10 := q[10] - r[10]
	d11 := q[11] - r[11]
	d12 := q[12] - r[12]
	d13 := q[13] - r[13]
	return d0*d0 + d1*d1 + d2*d2 + d3*d3 + d4*d4 + d5*d5 + d6*d6 + d7*d7 +
		d8*d8 + d9*d9 + d10*d10 + d11*d11 + d12*d12 + d13*d13
}

func (idx *Index) scanChunk(start, end int, q *[VectorDim]float32) topKResult {
	var res topKResult
	for i := start; i < end; i++ {
		off := i * VectorDim
		d := sqDist14(q, idx.vectors[off:off+VectorDim])
		res.insert(d, idx.labels[i])
	}
	return res
}

// Score runs a sharded parallel KNN search and returns the fraud fraction
// among the TopK nearest neighbors (0.0–1.0).
func (idx *Index) Score(query [VectorDim]float32) float32 {
	if idx.n == 0 {
		return 0
	}

	workers := runtime.GOMAXPROCS(0)
	if workers < 1 {
		workers = 1
	}
	if workers > idx.n {
		workers = idx.n
	}

	chunkSize := (idx.n + workers - 1) / workers
	results := make([]topKResult, workers)

	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		start := w * chunkSize
		end := start + chunkSize
		if end > idx.n {
			end = idx.n
		}
		wg.Add(1)
		go func(w, start, end int) {
			defer wg.Done()
			results[w] = idx.scanChunk(start, end, &query)
		}(w, start, end)
	}
	wg.Wait()

	// Merge each worker's top-K into a global top-K.
	var global topKResult
	for _, r := range results {
		for j := 0; j < r.size; j++ {
			global.insert(r.dists[j], r.labels[j])
		}
	}

	if global.size == 0 {
		return 0
	}
	var frauds int
	for j := 0; j < global.size; j++ {
		if global.labels[j] == 1 {
			frauds++
		}
	}
	return float32(frauds) / float32(global.size)
}
