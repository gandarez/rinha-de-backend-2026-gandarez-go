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
	"syscall"
	"time"

	"github.com/gandarez/rinha-de-backend-2026-gandarez-go/internal/fraud"
	"github.com/gandarez/rinha-de-backend-2026-gandarez-go/internal/handler"
)

const (
	defaultPort        = "9999"
	defaultResourceDir = "./resources"

	readHeaderTimeout = 5 * time.Second
	readTimeout       = 10 * time.Second
	writeTimeout      = 10 * time.Second
	idleTimeout       = 60 * time.Second
	shutdownTimeout   = 10 * time.Second
)

func main() {
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

	refPath := filepath.Join(resourcesDir, "references.json.gz")
	logger.Info("loading references", "path", refPath)
	t0 := time.Now()
	index, err := fraud.LoadReferences(refPath)
	if err != nil {
		logger.Error("load references", "err", err)
		os.Exit(1)
	}
	logger.Info("references loaded", "count", index.Len(), "elapsed", time.Since(t0))

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
