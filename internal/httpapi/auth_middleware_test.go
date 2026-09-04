package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"webmcp-backend/internal/auth"
)

type testPrincipalAuthorizer struct{ err error }

func (a testPrincipalAuthorizer) AuthorizePrincipal(context.Context, auth.Principal) error {
	return a.err
}

func TestProtectedRouteRequiresMembershipAuthorizerInProduction(t *testing.T) {
	c := testServerConfig()
	c.Environment = "production"
	server := NewServerWithDependencies(c, testLogger(), Dependencies{
		Authenticator: testAuthenticator{principal: apiPrincipal(auth.RoleViewer)},
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/businesses/biz-1/snapshot", nil)
	response := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), `"auth_not_ready"`) {
		t.Fatalf("production route trusted missing membership adapter: %d %s", response.Code, response.Body.String())
	}
}

func TestProtectedRouteRejectsUnmappedPrincipal(t *testing.T) {
	server := NewServerWithDependencies(testServerConfig(), testLogger(), Dependencies{
		Authenticator: testAuthenticator{principal: apiPrincipal(auth.RoleOwner)},
		Authorizer:    testPrincipalAuthorizer{err: auth.ErrPrincipalNotAuthorized},
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/businesses/biz-1/snapshot", nil)
	response := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || !strings.Contains(response.Body.String(), `"principal_not_authorized"`) {
		t.Fatalf("unmapped principal was accepted: %d %s", response.Code, response.Body.String())
	}
}

func TestProtectedRouteFailsClosedWhenMembershipStoreIsUnavailable(t *testing.T) {
	server := NewServerWithDependencies(testServerConfig(), testLogger(), Dependencies{
		Authenticator: testAuthenticator{principal: apiPrincipal(auth.RoleOwner)},
		Authorizer:    testPrincipalAuthorizer{err: auth.ErrPrincipalAuthorizationUnavailable},
	})
	request := httptest.NewRequest(http.MethodGet, "/api/v1/businesses/biz-1/snapshot", nil)
	response := httptest.NewRecorder()
	server.httpServer.Handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), `"auth_unavailable"`) {
		t.Fatalf("membership outage did not fail closed: %d %s", response.Code, response.Body.String())
	}
}
