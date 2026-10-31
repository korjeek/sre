package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"sre-incident-tracker/internal/logger"
	"sre-incident-tracker/internal/prober"
)

func main() {
	log := logger.New(getenv("LOG_LEVEL", "info"), "sre-prober")

	cfg := prober.Config{
		BaseURL:       strings.TrimRight(getenv("PROBE_TARGET_URL", "http://api.sre.svc.cluster.local"), "/"),
		Interval:      duration(log, "PROBE_INTERVAL", 5*time.Second),
		Timeout:       duration(log, "PROBE_TIMEOUT", 2*time.Second),
		SlowThreshold: duration(log, "PROBE_SLOW_THRESHOLD", 500*time.Millisecond),
		Targets:       prober.DefaultTargets(),
	}
	port := getenv("METRICS_PORT", "9091")

	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)
	p := prober.New(cfg, &http.Client{}, reg)

	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{Registry: reg}))
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	log.Info("starting prober",
		"target", cfg.BaseURL, "interval", cfg.Interval.String(),
		"timeout", cfg.Timeout.String(), "slow_threshold", cfg.SlowThreshold.String(), "port", port)

	go p.Run(ctx)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("metrics server failed", "err", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	log.Info("stopped")
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func duration(log interface{ Warn(string, ...any) }, key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		log.Warn("invalid duration, using default", "key", key, "value", v, "default", fallback.String())
		return fallback
	}
	return d
}
