package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"sre-incident-tracker/internal/config"
	"sre-incident-tracker/internal/db"
	"sre-incident-tracker/internal/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		panic(err)
	}
	log := logger.New(cfg.LogLevel)

	dir := getenv("MIGRATIONS_DIR", "migrations")
	ctx := context.Background()
	pool, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("database connect failed", "err", err)
		os.Exit(1)
	}
	defer pool.Close()

	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			filename TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		log.Error("cannot create schema_migrations", "err", err)
		os.Exit(1)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		log.Error("cannot read migrations dir", "err", err)
		os.Exit(1)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, name := range files {
		var exists bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE filename = $1)`, name).Scan(&exists); err != nil {
			log.Error("migration check failed", "file", name, "err", err)
			os.Exit(1)
		}
		if exists {
			log.Info("skip migration", "file", name)
			continue
		}
		body, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			log.Error("cannot read migration", "file", name, "err", err)
			os.Exit(1)
		}
		tx, err := pool.Begin(ctx)
		if err != nil {
			log.Error("begin tx failed", "err", err)
			os.Exit(1)
		}
		if _, err := tx.Exec(ctx, string(body)); err != nil {
			_ = tx.Rollback(ctx)
			log.Error("migration failed", "file", name, "err", err)
			os.Exit(1)
		}
		if _, err := tx.Exec(ctx, `INSERT INTO schema_migrations (filename) VALUES ($1)`, name); err != nil {
			_ = tx.Rollback(ctx)
			log.Error("cannot record migration", "file", name, "err", err)
			os.Exit(1)
		}
		if err := tx.Commit(ctx); err != nil {
			log.Error("commit failed", "file", name, "err", err)
			os.Exit(1)
		}
		log.Info("applied migration", "file", name)
	}
	fmt.Println("migrations complete")
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
