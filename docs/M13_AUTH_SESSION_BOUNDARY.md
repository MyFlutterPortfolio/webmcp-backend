# M13 — Authentication and Session Boundary

## Contract

Northstar never accepts identity, organization or role from browser payloads. Protected API requests require a bearer token validated by the backend. The configured OIDC issuer must publish RS256 keys and tokens must contain `iss`, `aud`, `sub`, `organization_id`, `role` and `exp` claims. `nbf` and `iat` are checked when present.

After signature and claim validation, every protected request performs a
server-side membership check in PostgreSQL. The `sub`, `organization_id` and
`role` values must match one `users` row; a signed token is not accepted as a
standalone authorization decision. Membership-store failure fails closed.

JWKS keys are cached for ten minutes and refreshed when an unknown `kid` is observed, supporting normal key rotation without accepting an unverified token.

## Failure posture

Missing or malformed authentication configuration keeps readiness false and protected routes return `auth_not_configured` until a real verifier is wired. Invalid, expired, wrong-audience, wrong-issuer, wrong-algorithm, unknown-key and invalid-role tokens return the same generic `unauthenticated` response. Token material is not logged or returned.

Production and staging require `WEBMCP_JWT_ISSUER`, `WEBMCP_JWT_AUDIENCE` and `WEBMCP_JWKS_URL`; production requires HTTPS for issuer and JWKS. Development/test may point JWKS at a local HTTP test server.

For a real user, provision `users.id` with the exact OIDC `sub`, the claimed
organization and the intended role before issuing access. Only `owner` and
`operator` membership can approve or commit. Guest mode uses a pre-seeded
viewer row and remains permanently unable to approve or commit.

## Frontend boundary

The frontend receives an in-memory `SessionHost` from the embedding product through `window.__WEBMCP_SESSION__`. The host owns refresh and token lifetime. The client attaches a token only for the current request, never persists it, never decodes it and converts a 401 into an expired-session state. Without a host, the app remains useful in preview mode while live API calls fail closed.
