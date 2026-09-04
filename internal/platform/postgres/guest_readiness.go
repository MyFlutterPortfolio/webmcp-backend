package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"webmcp-backend/internal/domain/core"
)

var ErrGuestDemoNotReady = errors.New("guest demo data is not ready")

// CheckGuestDemo verifies the complete read-only demo anchor in the same
// tenant-scoped transaction used by repositories. Readiness must not report a
// deploy as healthy when the configured demo token would point at missing data.
func CheckGuestDemo(ctx context.Context, pool *pgxpool.Pool, organizationID, userID, businessID, goalID core.ID) error {
	var ready bool
	err := WithTenantTx(ctx, pool, organizationID, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1
				FROM users u
				JOIN businesses b ON b.organization_id = u.organization_id
				JOIN goals g ON g.organization_id = b.organization_id AND g.business_id = b.id
				WHERE u.id = $1
				  AND u.role = 'viewer'
				  AND b.id = $2
				  AND g.id = $3
				  AND g.status = 'active'
			)
			AND EXISTS (
				SELECT 1 FROM products p
				WHERE p.organization_id = $4 AND p.business_id = $2 AND p.active = true
			)`, userID, businessID, goalID, organizationID).Scan(&ready)
	})
	if err != nil {
		return err
	}
	if !ready {
		return ErrGuestDemoNotReady
	}
	return nil
}
