package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Port        string
	MetricsPort string
	DatabaseURL string
	LogLevel    string
	StaticDir   string

	ChaosErrorRate float64
	ChaosLatency   time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		Port:        getenv("PORT", "8080"),
		MetricsPort: getenv("METRICS_PORT", "9090"),
		DatabaseURL: os.Getenv("DATABASE_URL"),
		LogLevel:    strings.ToLower(getenv("LOG_LEVEL", "info")),
		StaticDir:   getenv("STATIC_DIR", "web"),
	}
	if v := os.Getenv("CHAOS_ERROR_RATE"); v != "" {
		rate, err := strconv.ParseFloat(v, 64)
		if err != nil || rate < 0 || rate > 1 {
			return Config{}, fmt.Errorf("CHAOS_ERROR_RATE must be a number in [0,1], got %q", v)
		}
		cfg.ChaosErrorRate = rate
	}
	if v := os.Getenv("CHAOS_LATENCY_MS"); v != "" {
		ms, err := strconv.Atoi(v)
		if err != nil || ms < 0 {
			return Config{}, fmt.Errorf("CHAOS_LATENCY_MS must be a non-negative integer, got %q", v)
		}
		cfg.ChaosLatency = time.Duration(ms) * time.Millisecond
	}
	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
