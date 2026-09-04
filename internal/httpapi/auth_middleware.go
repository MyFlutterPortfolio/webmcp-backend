package httpapi

import (
	"context"
	"errors"
	"net/http"

	"webmcp-backend/internal/auth"
)

func authenticated(authenticator auth.Authenticator, authorizer PrincipalAuthorizer, requireAuthorizer bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if authenticator == nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="webmcp"`)
			writeError(w, r, http.StatusServiceUnavailable, "auth_not_configured", "authentication is not configured")
			return
		}
		principal, err := authenticator.Authenticate(r.Context(), r)
		if err != nil {
			status := http.StatusUnauthorized
			code := "unauthenticated"
			message := "authentication failed"
			if errors.Is(err, auth.ErrNotConfigured) {
				status = http.StatusServiceUnavailable
				code = "auth_not_configured"
				message = "authentication is not configured"
			}
			if status == http.StatusUnauthorized {
				w.Header().Set("WWW-Authenticate", `Bearer realm="webmcp"`)
			}
			writeError(w, r, status, code, message)
			return
		}
		if authorizer == nil && requireAuthorizer {
			// Production and staging wiring must include the database-backed
			// membership adapter; never silently trust token role claims there.
			writeError(w, r, http.StatusServiceUnavailable, "auth_not_ready", "authorization service is not ready")
			return
		}
		if authorizer != nil {
			if err := authorizer.AuthorizePrincipal(r.Context(), principal); err != nil {
				status, code, message := http.StatusForbidden, "principal_not_authorized", "authenticated principal is not authorized"
				if errors.Is(err, auth.ErrPrincipalAuthorizationUnavailable) {
					status, code, message = http.StatusServiceUnavailable, "auth_unavailable", "authorization service is temporarily unavailable"
				}
				writeError(w, r, status, code, message)
				return
			}
		}
		ctx, err := auth.WithPrincipal(r.Context(), principal)
		if err != nil {
			w.Header().Set("WWW-Authenticate", `Bearer realm="webmcp"`)
			writeError(w, r, http.StatusUnauthorized, "invalid_principal", "authenticated principal is invalid")
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func principalFromRequest(ctx context.Context) (auth.Principal, bool) {
	return auth.PrincipalFromContext(ctx)
}
