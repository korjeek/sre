package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"sre-incident-tracker/internal/chaos"
	"sre-incident-tracker/internal/metrics"
	"syscall"
	"time"

	"sre-incident-tracker/internal/config"
	"sre-incident-tracker/internal/db"
	"sre-incident-tracker/internal/httpapi"
	"sre-incident-tracker/internal/logger"
	"sre-incident-tracker/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	log := logger.New(cfg.LogLevel, "sre-api")
	log.Info("starting api", "port", cfg.Port)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("database connect failed", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	st := store.New(pool)
	appMetrics := metrics.New(st, log)

	chaosCfg := chaos.Config{ErrorRate: cfg.ChaosErrorRate, Latency: cfg.ChaosLatency}
	if chaosCfg.Enabled() {
		log.Warn("chaos mode enabled", "error_rate", chaosCfg.ErrorRate, "latency", chaosCfg.Latency.String())
	}

	handler := httpapi.New(st, log, cfg.StaticDir, appMetrics, chaosCfg)
	srv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	metricsMux := http.NewServeMux()
	metricsMux.Handle("GET /metrics", appMetrics.Handler())
	metricsSrv := &http.Server{
		Addr:              ":" + cfg.MetricsPort,
		Handler:           metricsMux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
	}

	errCh := make(chan error, 2)
	go func() {
		log.Info("listening", "addr", srv.Addr)
		errCh <- srv.ListenAndServe()
	}()
	go func() {
		log.Info("metrics listening", "addr", metricsSrv.Addr)
		errCh <- metricsSrv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		log.Info("shutdown signal received")
	case err := <-errCh:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("server error", "err", err)
			os.Exit(1)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed", "err", err)
		os.Exit(1)
	}
	log.Info("stopped")
}
