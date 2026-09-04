package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"webmcp-backend/internal/auth"
)

// PrincipalAuthorizer is the server-side membership boundary for verified
// OIDC subjects. The token supplies identity evidence; PostgreSQL supplies
// current tenant membership and role authority.
type PrincipalAuthorizer struct {
	Pool *pgxpool.Pool
}

func (a PrincipalAuthorizer) AuthorizePrincipal(ctx context.Context, principal auth.Principal) error {
	if err := principal.Validate(); err != nil {
		return auth.ErrPrincipalNotAuthorized
	}
	if a.Pool == nil {
		return auth.ErrPrincipalAuthorizationUnavailable
	}
	err := WithTenantTx(ctx, a.Pool, principal.OrganizationID, func(ctx context.Context, tx pgx.Tx) error {
		var role string
		err := tx.QueryRow(ctx, `
			SELECT role
			FROM users
			WHERE id = $1 AND organization_id = $2`, principal.UserID, principal.OrganizationID).Scan(&role)
		if errors.Is(err, pgx.ErrNoRows) {
			return auth.ErrPrincipalNotAuthorized
		}
		if err != nil {
			return fmt.Errorf("%w: membership lookup failed: %v", auth.ErrPrincipalAuthorizationUnavailable, err)
		}
		if role != string(principal.Role) {
			return auth.ErrPrincipalNotAuthorized
		}
		return nil
	})
	if err == nil || errors.Is(err, auth.ErrPrincipalNotAuthorized) || errors.Is(err, auth.ErrPrincipalAuthorizationUnavailable) {
		return err
	}
	return fmt.Errorf("%w: membership transaction failed: %v", auth.ErrPrincipalAuthorizationUnavailable, err)
}
