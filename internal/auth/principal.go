package auth

import (
	"context"
	"errors"
	"fmt"

	"webmcp-backend/internal/domain/core"
)

var (
	// ErrPrincipalNotAuthorized means the cryptographically authenticated
	// subject is not an active member of the claimed organization with the
	// claimed role. It is deliberately generic so callers cannot enumerate
	// users or tenants.
	ErrPrincipalNotAuthorized = errors.New("principal is not authorized")
	// ErrPrincipalAuthorizationUnavailable means the membership check could
	// not be completed safely. Protected requests must fail closed in this
	// case rather than falling back to token claims.
	ErrPrincipalAuthorizationUnavailable = errors.New("principal authorization is unavailable")
)

type contextKey struct{}
type guestContextKey struct{}

type Role string

const (
	RoleOwner    Role = "owner"
	RoleOperator Role = "operator"
	RoleViewer   Role = "viewer"
)

type Principal struct {
	UserID          core.ID
	OrganizationID  core.ID
	Role            Role
	Guest           bool
	GuestBusinessID core.ID
	GuestGoalID     core.ID
}

func (p Principal) Validate() error {
	if err := p.UserID.Validate("user_id"); err != nil {
		return err
	}
	if err := p.OrganizationID.Validate("organization_id"); err != nil {
		return err
	}
	if p.Role != RoleOwner && p.Role != RoleOperator && p.Role != RoleViewer {
		return fmt.Errorf("invalid role %q", p.Role)
	}
	if p.Guest {
		if err := p.GuestBusinessID.Validate("guest_business_id"); err != nil {
			return err
		}
		if err := p.GuestGoalID.Validate("guest_goal_id"); err != nil {
			return err
		}
		if p.Role != RoleViewer {
			return fmt.Errorf("guest principals must be viewers")
		}
	}
	return nil
}

func (p Principal) CanApprove() bool {
	return p.Role == RoleOwner || p.Role == RoleOperator
}

func (p Principal) CanOperate() bool {
	return p.Role == RoleOwner || p.Role == RoleOperator
}

// CanDemoOperate permits only non-canonical scenario/proposal work in the
// short-lived guest environment. Approval, constraints and commit remain
// unavailable because those methods intentionally do not include this path.
func (p Principal) CanDemoOperate() bool {
	return p.Guest && p.Role == RoleViewer
}

func (p Principal) CanCommit() bool {
	return p.Role == RoleOwner || p.Role == RoleOperator
}

func WithPrincipal(ctx context.Context, principal Principal) (context.Context, error) {
	if err := principal.Validate(); err != nil {
		return ctx, err
	}
	ctx = context.WithValue(ctx, contextKey{}, principal)
	return context.WithValue(ctx, guestContextKey{}, principal.Guest), nil
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(contextKey{}).(Principal)
	return principal, ok
}

// IsGuest reports the authenticated mode attached by WithPrincipal. It is
// used by persistence adapters to preserve the bounded demo capability while
// keeping the public role viewer-only.
func IsGuest(ctx context.Context) bool {
	guest, _ := ctx.Value(guestContextKey{}).(bool)
	return guest
}

func (p Principal) GuestAllows(businessID, goalID core.ID) bool {
	return !p.Guest || (businessID == p.GuestBusinessID && (goalID == "" || goalID == p.GuestGoalID))
}
