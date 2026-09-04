# M6 — WebMCP/API Integration

## Objective

Connect the browser-native WebMCP capability graph to authoritative backend boundaries without turning the backend into a browser MCP transport. The browser registers and executes tools; the API authenticates the human session, enforces tenancy and performs the durable business operation.

## Implemented boundaries

### `GET /api/v1/businesses/{businessId}/snapshot`

- requires a configured authenticator;
- resolves organization scope from the authenticated principal, never from client input;
- reads the authoritative versioned snapshot through the repository port;
- returns explicit lower-case response DTOs, including empty arrays instead of null collections;
- returns a stable `request_id` for activity correlation.

### `POST /api/v1/proposals/{proposalId}/commit`

- requires an owner or operator principal before reaching the commit port;
- requires `Idempotency-Key` with bounded length;
- requires both `business_id` and the exact `proposal_version_id` in the JSON body;
- rejects unknown fields and trailing JSON values;
- derives a deterministic operation identity from organization and idempotency key;
- forwards authorization, actor, tenant and request-correlation context to the transactional commit service;
- maps domain safety failures to stable HTTP error codes without exposing internal errors.

The database commit service remains the final authority. It revalidates role, business ownership, proposal version, human approval, optimistic version and idempotency inside one tenant-scoped transaction.

## Fail-closed authentication

The HTTP server does not ship a demo authenticator and never trusts a frontend-supplied user or organization ID. A provider-neutral OIDC JWT/JWKS adapter now validates configured issuer, audience, signature, tenant and role claims. Until it is configured with the deployed identity provider, `/api/v1/*` returns `503 auth_not_configured`. This is intentional: deployment must add identity verification, not bypass it.

## Verification

- backend: `gofmt -w cmd internal`, `go test ./...`, `go vet ./...`, `go build ./...`;
- frontend: `npm run typecheck`, `npm run test`, `npm run build`;
- pending external boundary: live PostgreSQL migration/RLS test, real identity-provider adapter, deployed HTTPS origin and WebMCP execution in ChatGPT/Chromium.
