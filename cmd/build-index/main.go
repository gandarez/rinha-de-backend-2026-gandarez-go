// build-index runs offline k-means on references.json.gz and writes a binary IVF
// index (index.bin.gz) consumed by the runtime API. Called from download-resources.sh
// during Docker image build.
package main

import (
	"compress/gzip"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"math/rand"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/gandarez/rinha-de-backend-2026-gandarez-go/internal/fraud"
)

const (
	numClusters = 1024
	numIter     = 15
	quantScale  = int16(10000)
)

func main() {
	inPath := flag.String("in", "./resources/references.json.gz", "path to references.json.gz")
	outPath := flag.String("out", "./resources/index.bin.gz", "path to write index.bin.gz")
	flag.Parse()

	log.Printf("loading references from %s", *inPath)
	t0 := time.Now()
	idx, err := fraud.LoadReferences(*inPath)
	if err != nil {
		log.Fatalf("load references: %v", err)
	}
	n := idx.Len()
	log.Printf("loaded %d vectors in %v", n, time.Since(t0))

	vectors, labels := idx.RawData()

	log.Printf("running k-means: k=%d iterations=%d workers=%d", numClusters, numIter, runtime.NumCPU())
	t0 = time.Now()
	centroids, assignments := kmeans(vectors, n, numClusters, numIter)
	log.Printf("k-means done in %v", time.Since(t0))

	log.Printf("writing index to %s", *outPath)
	if err := writeIndex(*outPath, n, centroids, assignments, vectors, labels); err != nil {
		log.Fatalf("write index: %v", err)
	}
	log.Println("done")
}

// kmeans runs Lloyd's k-means and returns centroids (flat float32) and per-vector assignments.
func kmeans(vectors []float32, n, k, maxIter int) ([]float32, []int32) {
	dim := fraud.VectorDim

	// Init: random sample of k distinct points
	rng := rand.New(rand.NewSource(42))
	centroids := make([]float32, k*dim)
	perm := rng.Perm(n)
	for c := 0; c < k; c++ {
		vi := perm[c] * dim
		copy(centroids[c*dim:c*dim+dim], vectors[vi:vi+dim])
	}

	assignments := make([]int32, n)
	workers := runtime.NumCPU()
	if workers < 1 {
		workers = 1
	}

	for iter := 0; iter < maxIter; iter++ {
		// --- Assignment step (parallel) ---
		parallelAssign(vectors, centroids, assignments, n, k, workers)

		// --- Update step: recompute centroids as cluster means ---
		sums := make([]float64, k*dim)
		counts := make([]int64, k)
		for i := 0; i < n; i++ {
			c := int(assignments[i])
			counts[c]++
			vi := i * dim
			ci := c * dim
			for d := 0; d < dim; d++ {
				sums[ci+d] += float64(vectors[vi+d])
			}
		}
		for c := 0; c < k; c++ {
			cnt := counts[c]
			if cnt == 0 {
				// Empty cluster: reinit to a random point.
				vi := rng.Intn(n) * dim
				copy(centroids[c*dim:c*dim+dim], vectors[vi:vi+dim])
				continue
			}
			ci := c * dim
			for d := 0; d < dim; d++ {
				centroids[ci+d] = float32(sums[ci+d] / float64(cnt))
			}
		}
		log.Printf("  iter %d/%d", iter+1, maxIter)
	}

	return centroids, assignments
}

// parallelAssign assigns each vector to its nearest centroid using multiple goroutines.
func parallelAssign(vectors, centroids []float32, assignments []int32, n, k, workers int) {
	dim := fraud.VectorDim
	chunkSize := (n + workers - 1) / workers
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		start := w * chunkSize
		end := start + chunkSize
		if end > n {
			end = n
		}
		wg.Add(1)
		go func(start, end int) {
			defer wg.Done()
			var q [fraud.VectorDim]float32
			for i := start; i < end; i++ {
				copy(q[:], vectors[i*dim:i*dim+dim])
				best := int32(0)
				bestDist := float32(math.MaxFloat32)
				for c := 0; c < k; c++ {
					d := fraud.SqDist14(&q, centroids[c*dim:c*dim+dim])
					if d < bestDist {
						bestDist = d
						best = int32(c)
					}
				}
				assignments[i] = best
			}
		}(start, end)
	}
	wg.Wait()
}

// writeIndex writes the binary IVF index as a gzip-compressed file.
func writeIndex(path string, n int, centroids []float32, assignments []int32, vectors []float32, labels []uint8) error {
	dim := fraud.VectorDim
	k := numClusters

	// Compute cluster sizes and sort order (group vectors by cluster).
	sizes := make([]uint32, k)
	for _, c := range assignments {
		sizes[c]++
	}
	starts := make([]uint32, k)
	for c := 1; c < k; c++ {
		starts[c] = starts[c-1] + sizes[c-1]
	}

	// Build sorted index: order[i] = original vector index, grouped by cluster.
	order := make([]int32, n)
	pos := make([]uint32, k)
	copy(pos, starts)
	for i, c := range assignments {
		order[pos[c]] = int32(i)
		pos[c]++
	}

	// Write quantized vectors and labels in cluster order.
	qVectors := make([]int16, n*dim)
	sortedLabels := make([]uint8, n)
	for out, orig := range order {
		vi := int(orig) * dim
		oi := out * dim
		for d := 0; d < dim; d++ {
			qVectors[oi+d] = int16(vectors[vi+d] * float32(quantScale))
		}
		sortedLabels[out] = labels[orig]
	}

	// Open output file and gzip writer.
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewWriterLevel(f, gzip.BestSpeed)
	if err != nil {
		return fmt.Errorf("gzip writer: %w", err)
	}
	defer gz.Close()

	if err := writeAll(gz, n, k, centroids, sizes, starts, qVectors, sortedLabels); err != nil {
		return err
	}
	return gz.Close()
}

func writeAll(w io.Writer, n, k int, centroids []float32, sizes, starts []uint32, qVectors []int16, labels []uint8) error {
	bw := binary.LittleEndian
	// magic
	if _, err := w.Write([]byte("IVF1")); err != nil {
		return err
	}
	// header
	hdr := [5]uint32{uint32(n), uint32(k), uint32(fraud.VectorDim), uint32(quantScale), 0}
	if err := binary.Write(w, bw, hdr); err != nil {
		return err
	}
	// centroids
	if err := binary.Write(w, bw, centroids); err != nil {
		return err
	}
	// sizes
	if err := binary.Write(w, bw, sizes); err != nil {
		return err
	}
	// starts
	if err := binary.Write(w, bw, starts); err != nil {
		return err
	}
	// quantized vectors
	if err := binary.Write(w, bw, qVectors); err != nil {
		return err
	}
	// labels
	_, err := w.Write(labels)
	return err
}
