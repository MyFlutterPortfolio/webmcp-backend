package postgres

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"webmcp-backend/internal/domain/core"
)

func TestTenantIDValidationIsFailClosed(t *testing.T) {
	if err := core.ID("").Validate("organization_id"); err == nil {
		t.Fatal("expected empty tenant id to be rejected")
	}
}

func TestWithTenantTxFailsClosedWhenDatabasePoolIsMissing(t *testing.T) {
	err := WithTenantTx(context.Background(), nil, core.ID("org-1"), func(context.Context, pgx.Tx) error {
		t.Fatal("transaction callback must not run without a database pool")
		return nil
	})
	if !errors.Is(err, ErrDatabaseUnavailable) {
		t.Fatalf("error = %v, want ErrDatabaseUnavailable", err)
	}
}
