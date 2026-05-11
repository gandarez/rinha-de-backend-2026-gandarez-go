package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/gandarez/rinha-de-backend-2026-gandarez-go/internal/fraud"
	"github.com/gandarez/rinha-de-backend-2026-gandarez-go/internal/handler"
)

const (
	defaultPort        = "9999"
	defaultResourceDir = "./resources"

	readHeaderTimeout = 2 * time.Second
	readTimeout       = 2 * time.Second
	writeTimeout      = 2 * time.Second
	idleTimeout       = 5 * time.Second
	shutdownTimeout   = 5 * time.Second
)

func main() {
	// With near-zero per-request allocation (sync.Pool), a higher GC target
	// reduces GC frequency without risking OOM under the 140MiB container limit.
	debug.SetGCPercent(500)

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	resourcesDir := os.Getenv("RESOURCES_DIR")
	if resourcesDir == "" {
		resourcesDir = defaultResourceDir
	}

	constants, err := fraud.LoadConstants(filepath.Join(resourcesDir, "normalization.json"))
	if err != nil {
		logger.Error("load normalization constants", "err", err)
		os.Exit(1)
	}

	mccRisk, err := fraud.LoadMCCRisk(filepath.Join(resourcesDir, "mcc_risk.json"))
	if err != nil {
		logger.Error("load mcc_risk", "err", err)
		os.Exit(1)
	}

	t0 := time.Now()
	var index fraud.Indexer

	ivfPath := filepath.Join(resourcesDir, "index.bin.gz")
	if ivf, err := fraud.LoadIVF(ivfPath); err == nil {
		logger.Info("ivf index loaded", "path", ivfPath, "elapsed", time.Since(t0))
		index = ivf
	} else {
		// Fallback to brute-force KNN when no pre-built IVF index exists.
		logger.Info("ivf index not found, falling back to brute-force KNN", "reason", err)
		t0 = time.Now()
		refPath := filepath.Join(resourcesDir, "references.json.gz")
		bf, err := fraud.LoadReferences(refPath)
		if err != nil {
			logger.Error("load references", "err", err)
			os.Exit(1)
		}
		logger.Info("references loaded", "count", bf.Len(), "elapsed", time.Since(t0))
		index = bf
	}

	scorer := fraud.New(fraud.NewVectorizer(constants, mccRisk), index)
	fraudHandler := handler.New(scorer)

	port := os.Getenv("PORT")
	if port == "" {
		port = defaultPort
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ready", handler.Ready)
	mux.HandleFunc("POST /fraud-score", fraudHandler.Score)

	srv := &http.Server{
		Addr:              net.JoinHostPort("", port),
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("server starting", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case err := <-serverErr:
		if err != nil {
			logger.Error("server failed", "err", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", "err", err)
		os.Exit(1)
	}

	logger.Info("server stopped")
}
