package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"webmcp-backend/internal/domain/core"
)

type TxFunc func(context.Context, pgx.Tx) error
type TxResultFunc[T any] func(context.Context, pgx.Tx) (T, error)

var ErrDatabaseUnavailable = errors.New("database is unavailable")

// WithTenantTx sets the tenant context inside the transaction. SET LOCAL
// guarantees the value cannot leak to another pooled transaction.
func WithTenantTx(ctx context.Context, pool *pgxpool.Pool, organizationID core.ID, fn TxFunc) error {
	_, err := WithTenantTxResult(ctx, pool, organizationID, func(ctx context.Context, tx pgx.Tx) (struct{}, error) {
		return struct{}{}, fn(ctx, tx)
	})
	return err
}

func WithTenantTxResult[T any](ctx context.Context, pool *pgxpool.Pool, organizationID core.ID, fn TxResultFunc[T]) (T, error) {
	var zero T
	if err := organizationID.Validate("organization_id"); err != nil {
		return zero, err
	}
	if pool == nil {
		return zero, ErrDatabaseUnavailable
	}
	if fn == nil {
		return zero, fmt.Errorf("transaction callback is required")
	}
	tx, err := pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return zero, fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `SELECT set_config('app.organization_id', $1, true)`, string(organizationID)); err != nil {
		return zero, fmt.Errorf("set tenant transaction context: %w", err)
	}
	result, err := fn(ctx, tx)
	if err != nil {
		return zero, err
	}
	if err := tx.Commit(ctx); err != nil {
		return zero, fmt.Errorf("commit transaction: %w", err)
	}
	return result, nil
}
