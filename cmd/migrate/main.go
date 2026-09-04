package main

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/jackc/pgx/v5"

	"webmcp-backend/internal/platform/postgres"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	databaseURL := os.Getenv("WEBMCP_DATABASE_URL")
	if databaseURL == "" {
		logger.Error("migration failed", "error", "WEBMCP_DATABASE_URL is not configured")
		os.Exit(1)
	}
	migrations, err := postgres.LoadMigrations(migrationDirectory())
	if err != nil {
		logger.Error("migration discovery failed", "error", err)
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		logger.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer conn.Close(context.Background())
	if err := postgres.ApplyMigrations(ctx, conn, migrations); err != nil {
		logger.Error("migration failed", "error", err)
		os.Exit(1)
	}
	logger.Info("database migrations applied", "count", len(migrations))
}

func migrationDirectory() string {
	if value := os.Getenv("WEBMCP_MIGRATIONS_DIR"); value != "" {
		return value
	}
	return filepath.Join(".", "migrations")
}
