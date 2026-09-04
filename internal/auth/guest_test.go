package auth

import (
	"net/http/httptest"
	"testing"
	"time"

	"webmcp-backend/internal/domain/core"
)

func TestGuestTokenIsShortLivedScopedAndViewerOnly(t *testing.T) {
	authenticator, err := NewGuestAuthenticator("01234567890123456789012345678901", "user-demo", "org-demo", "business-demo", "goal-demo", 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := authenticator.IssueGuestToken()
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	principal, err := authenticator.Authenticate(nil, request)
	if err != nil {
		t.Fatal(err)
	}
	if !principal.Guest || principal.Role != RoleViewer || principal.GuestBusinessID != core.ID("business-demo") || !principal.CanDemoOperate() {
		t.Fatalf("unexpected guest principal: %#v", principal)
	}
	if principal.CanApprove() || principal.CanCommit() || principal.CanOperate() || !principal.GuestAllows("business-demo", "goal-demo") || principal.GuestAllows("other-business", "goal-demo") {
		t.Fatal("guest principal crossed a protected permission or scope boundary")
	}
}

func TestGuestTokenRejectsTampering(t *testing.T) {
	authenticator, err := NewGuestAuthenticator("01234567890123456789012345678901", "user-demo", "org-demo", "business-demo", "goal-demo", 15*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := authenticator.IssueGuestToken()
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("GET", "/", nil)
	request.Header.Set("Authorization", "Bearer "+token+"x")
	if _, err := authenticator.Authenticate(nil, request); err == nil {
		t.Fatal("tampered guest token was accepted")
	}
}
