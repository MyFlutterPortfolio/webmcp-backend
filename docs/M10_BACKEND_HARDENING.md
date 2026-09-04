# M10 — Backend Hardening

## Security controls

- production uses PostgreSQL `sslmode=verify-full`; staging requires encrypted transport;
- every tenant table uses PostgreSQL RLS, and `FORCE ROW LEVEL SECURITY` prevents table-owner bypass;
- scenario/proposal/approval write repositories independently verify tenant membership and owner/operator role;
- scenario simulation rechecks the current business version in the same persistence transaction;
- API responses send `Cache-Control: no-store` and `Pragma: no-cache`;
- CORS accepts only configured exact origins, methods and headers;
- authentication remains fail-closed and unauthenticated responses carry a Bearer challenge;
- database sessions enforce statement, lock and idle-in-transaction timeouts.

## Operational controls

Readiness is not manually considered healthy until required dependencies are wired. When a database is configured, `/readyz` performs a bounded database ping. The process remains alive for diagnostics through `/healthz`, while protected API routes remain unavailable until authentication and business dependencies are present.

Scenario creation now carries the request correlation ID into its audit event. This preserves a trace from WebMCP invocation through working-state creation, review, approval and eventual commit.

## Verification boundary

Unit, HTTP contract, auth verifier, `go vet`, `go build`, frontend typecheck, frontend tests and frontend production build pass locally. Provider-specific identity claim mapping, live PostgreSQL migration/RLS/concurrency tests, container smoke tests and target-browser WebMCP testing remain deployment release gates.
