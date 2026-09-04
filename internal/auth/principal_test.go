package auth

import (
	"context"
	"testing"
)

func TestPrincipalRolePolicy(t *testing.T) {
	owner := Principal{UserID: "u-1", OrganizationID: "org-1", Role: RoleOwner}
	if !owner.CanApprove() || !owner.CanCommit() {
		t.Fatal("owner must approve and commit")
	}
	viewer := Principal{UserID: "u-2", OrganizationID: "org-1", Role: RoleViewer}
	if viewer.CanApprove() || viewer.CanCommit() {
		t.Fatal("viewer must not approve or commit")
	}
	ctx, err := WithPrincipal(context.Background(), owner)
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := PrincipalFromContext(ctx); !ok || got.UserID != owner.UserID {
		t.Fatal("principal was not recovered from context")
	}
}
