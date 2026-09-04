package postgres

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"webmcp-backend/internal/config"
)

func Open(ctx context.Context, cfg config.Config, logger *slog.Logger) (*pgxpool.Pool, error) {
	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("database URL is not configured")
	}
	poolConfig, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}
	poolConfig.MinConns = cfg.DBMinConns
	poolConfig.MaxConns = cfg.DBMaxConns
	if poolConfig.ConnConfig.RuntimeParams == nil {
		poolConfig.ConnConfig.RuntimeParams = make(map[string]string)
	}
	statementTimeout := cfg.RequestTimeout.Milliseconds()
	if statementTimeout < 1000 {
		statementTimeout = 1000
	}
	poolConfig.ConnConfig.RuntimeParams["statement_timeout"] = fmt.Sprintf("%d", statementTimeout)
	poolConfig.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = fmt.Sprintf("%d", statementTimeout)
	poolConfig.ConnConfig.RuntimeParams["lock_timeout"] = fmt.Sprintf("%d", minInt64(statementTimeout, 5000))
	poolConfig.AfterConnect = func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, `SET TIME ZONE 'UTC'`)
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create database pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	logger.InfoContext(ctx, "postgres connection established", "max_conns", cfg.DBMaxConns)
	return pool, nil
}

func minInt64(left, right int64) int64 {
	if left < right {
		return left
	}
	return right
}
