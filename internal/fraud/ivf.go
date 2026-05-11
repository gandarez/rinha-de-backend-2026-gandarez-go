package fraud

import (
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
)

const (
	QuantScale     = 10000
	DefaultProbes  = 8
	ExpandedProbes = 24
)

// IVFIndex is an Inverted File index: vectors are quantized to int16 and grouped
// by k-means cluster. A query probes a small number of clusters (DefaultProbes)
// and only re-probes more if the result is ambiguous.
type IVFIndex struct {
	centroids []float32 // nClusters * VectorDim
	sizes     []uint32  // nClusters
	starts    []uint32  // nClusters prefix-sum into qVectors/labels
	qVectors  []int16   // n * VectorDim, grouped by cluster
	labels    []uint8   // n
	nClusters int
}

// topKInt64 tracks the K nearest candidates with int64 distances (needed for
// quantized int16 arithmetic that can exceed float32 precision).
type topKInt64 struct {
	dists  [TopK]int64
	labels [TopK]uint8
	size   int
	maxIdx int
}

func (r *topKInt64) insert(d int64, label uint8) {
	if r.size < TopK {
		r.dists[r.size] = d
		r.labels[r.size] = label
		r.size++
		if r.size == TopK {
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

func countFraudsInt64(r *topKInt64) int {
	var frauds int
	for j := 0; j < r.size; j++ {
		if r.labels[j] == 1 {
			frauds++
		}
	}
	return frauds
}

// sqDistInt16x14 computes squared Euclidean distance between two 14-dim int16 vectors.
// Returns int64 to avoid overflow (max value: 14 * 20000^2 = 5.6e9 < 2^63).
func sqDistInt16x14(q *[VectorDim]int16, r []int16) int64 {
	_ = r[13]
	d0 := int64(q[0]) - int64(r[0])
	d1 := int64(q[1]) - int64(r[1])
	d2 := int64(q[2]) - int64(r[2])
	d3 := int64(q[3]) - int64(r[3])
	d4 := int64(q[4]) - int64(r[4])
	d5 := int64(q[5]) - int64(r[5])
	d6 := int64(q[6]) - int64(r[6])
	d7 := int64(q[7]) - int64(r[7])
	d8 := int64(q[8]) - int64(r[8])
	d9 := int64(q[9]) - int64(r[9])
	d10 := int64(q[10]) - int64(r[10])
	d11 := int64(q[11]) - int64(r[11])
	d12 := int64(q[12]) - int64(r[12])
	d13 := int64(q[13]) - int64(r[13])
	return d0*d0 + d1*d1 + d2*d2 + d3*d3 + d4*d4 + d5*d5 + d6*d6 + d7*d7 +
		d8*d8 + d9*d9 + d10*d10 + d11*d11 + d12*d12 + d13*d13
}

// nearestCentroids returns the indices of the nProbe closest centroids to q.
// Uses a fixed-size max-heap to avoid sorting the full nClusters results.
func (idx *IVFIndex) nearestCentroids(q *[VectorDim]float32, nProbe int) []int {
	type pair struct {
		dist float32
		idx  int
	}

	heap := make([]pair, 0, nProbe)
	maxDist := float32(math.MaxFloat32)
	maxPos := 0

	for c := 0; c < idx.nClusters; c++ {
		d := sqDist14(q, idx.centroids[c*VectorDim:(c+1)*VectorDim])
		if len(heap) < nProbe {
			heap = append(heap, pair{d, c})
			if len(heap) == nProbe {
				// Find worst in heap
				maxPos = 0
				maxDist = heap[0].dist
				for j := 1; j < nProbe; j++ {
					if heap[j].dist > maxDist {
						maxDist = heap[j].dist
						maxPos = j
					}
				}
			}
		} else if d < maxDist {
			heap[maxPos] = pair{d, c}
			maxPos = 0
			maxDist = heap[0].dist
			for j := 1; j < nProbe; j++ {
				if heap[j].dist > maxDist {
					maxDist = heap[j].dist
					maxPos = j
				}
			}
		}
	}

	result := make([]int, len(heap))
	for i, p := range heap {
		result[i] = p.idx
	}
	return result
}

// scanCluster scans all vectors in a cluster and updates the topK result.
func (idx *IVFIndex) scanCluster(clusterID int, q *[VectorDim]int16, res *topKInt64) {
	start := idx.starts[clusterID]
	size := idx.sizes[clusterID]
	end := start + size
	vs := idx.qVectors[start*VectorDim : end*VectorDim]
	ls := idx.labels[start:end]
	for i := uint32(0); i < size; i++ {
		d := sqDistInt16x14(q, vs[i*VectorDim:(i+1)*VectorDim])
		res.insert(d, ls[i])
	}
}

// Score returns the count of fraud neighbors (0..TopK) using two-phase IVF probing.
func (idx *IVFIndex) Score(query [VectorDim]float32) int {
	// Quantize query once.
	var qInt [VectorDim]int16
	for i, v := range query {
		qInt[i] = int16(v * QuantScale)
	}

	// Phase A: find nearest DefaultProbes centroids.
	probes := idx.nearestCentroids(&query, DefaultProbes)

	// Phase B: scan chosen clusters.
	var res topKInt64
	for _, c := range probes {
		idx.scanCluster(c, &qInt, &res)
	}
	frauds := countFraudsInt64(&res)

	// Phase C: ambiguity guard — re-probe with more clusters when result is borderline.
	// With K=5 and threshold 0.6, only counts 2 and 3 are ambiguous (flip approval).
	if frauds == 2 || frauds == 3 {
		probes2 := idx.nearestCentroids(&query, ExpandedProbes)
		var res2 topKInt64
		for _, c := range probes2 {
			idx.scanCluster(c, &qInt, &res2)
		}
		frauds = countFraudsInt64(&res2)
	}

	return frauds
}

// LoadIVF reads a binary IVF index produced by cmd/build-index from path.
func LoadIVF(path string) (*IVFIndex, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open ivf: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("gzip reader: %w", err)
	}
	defer gz.Close()

	return loadIVFFromReader(gz)
}

func loadIVFFromReader(r io.Reader) (*IVFIndex, error) {
	bw := binary.LittleEndian

	// Magic
	var magic [4]byte
	if _, err := io.ReadFull(r, magic[:]); err != nil {
		return nil, fmt.Errorf("read magic: %w", err)
	}
	if string(magic[:]) != "IVF1" {
		return nil, fmt.Errorf("invalid magic: %q", magic)
	}

	// Header: [n, k, dim, scale, _padding]
	var hdr [5]uint32
	if err := binary.Read(r, bw, &hdr); err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}
	n, k, dim := int(hdr[0]), int(hdr[1]), int(hdr[2])
	if dim != VectorDim {
		return nil, fmt.Errorf("dim mismatch: got %d want %d", dim, VectorDim)
	}

	// Centroids
	centroids := make([]float32, k*VectorDim)
	if err := binary.Read(r, bw, centroids); err != nil {
		return nil, fmt.Errorf("read centroids: %w", err)
	}

	// Sizes
	sizes := make([]uint32, k)
	if err := binary.Read(r, bw, sizes); err != nil {
		return nil, fmt.Errorf("read sizes: %w", err)
	}

	// Starts
	starts := make([]uint32, k)
	if err := binary.Read(r, bw, starts); err != nil {
		return nil, fmt.Errorf("read starts: %w", err)
	}

	// Quantized vectors
	qVectors := make([]int16, n*VectorDim)
	if err := binary.Read(r, bw, qVectors); err != nil {
		return nil, fmt.Errorf("read qvectors: %w", err)
	}

	// Labels
	labels := make([]uint8, n)
	if _, err := io.ReadFull(r, labels); err != nil {
		return nil, fmt.Errorf("read labels: %w", err)
	}

	return &IVFIndex{
		centroids: centroids,
		sizes:     sizes,
		starts:    starts,
		qVectors:  qVectors,
		labels:    labels,
		nClusters: k,
	}, nil
}
