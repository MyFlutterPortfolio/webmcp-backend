# Railway deployment gate

This service is designed to run as a Docker web service on Railway. The
process deliberately does not set `WEBMCP_HTTP_ADDR` in Railway: the server
uses Railway's injected `PORT` and binds to `0.0.0.0:<PORT>`.

For a local image smoke test, pass `PORT` (or an explicit public bind):

```powershell
docker build -t webmcp-backend:release-candidate .
docker run --rm -p 18080:8080 -e WEBMCP_ENV=development -e PORT=8080 webmcp-backend:release-candidate
```

Without `PORT`, the development default intentionally binds to loopback and
is not reachable through Docker port publishing.

## Required Railway variables

Set these in the Railway service Variables tab. Values containing secrets must
be entered in Railway, never committed to `.env.example` or copied into a
log:

```text
WEBMCP_ENV=production
WEBMCP_DATABASE_URL=postgresql://USER:PASSWORD@HOST/DATABASE?sslmode=verify-full
WEBMCP_ALLOWED_ORIGINS=https://<frontend-host>
WEBMCP_JWT_ISSUER=https://<identity-provider-issuer>
WEBMCP_JWT_AUDIENCE=webmcp-api
WEBMCP_JWKS_URL=https://<identity-provider-jwks-endpoint>
WEBMCP_DB_MIN_CONNS=1
WEBMCP_DB_MAX_CONNS=5
WEBMCP_REQUEST_TIMEOUT=15s
WEBMCP_MAX_BODY_BYTES=1048576
WEBMCP_AGENT_CHAT_ENABLED=true
WEBMCP_GROQ_API_KEY=<Railway secret>
WEBMCP_GROQ_FALLBACK_API_KEY=<Railway secret>
WEBMCP_GROQ_FALLBACK_API_KEY_2=<Railway secret>
WEBMCP_GROQ_MODEL=openai/gpt-oss-120b
WEBMCP_GROQ_FALLBACK_MODEL=openai/gpt-oss-120b
WEBMCP_GROQ_FALLBACK_MODEL_2=openai/gpt-oss-120b
WEBMCP_GROQ_BASE_URL=https://api.groq.com/openai/v1
WEBMCP_GROQ_MAX_OUTPUT_TOKENS=700
```

The three Groq secrets are isolated routing slots. The primary slot is tried
first, then fallback slot 1 and fallback slot 2; the deterministic guide is
the final safe fallback. Never place a Groq key in the frontend environment,
Vite variables, WebMCP tool code or committed files.

`WEBMCP_JWT_ISSUER`, `WEBMCP_JWT_AUDIENCE` and `WEBMCP_JWKS_URL` are a single
authentication boundary. The service stays unready until all three are
configured and the database is reachable. Protected requests additionally
verify that the token subject, organization and role match a PostgreSQL
`users` membership row; an arbitrary signed OIDC token is not sufficient.

## Judge-only guest demo mode

If an external identity provider is not available before judging, enable the
bounded guest demo instead of weakening the protected API:

```text
WEBMCP_GUEST_DEMO_ENABLED=true
WEBMCP_GUEST_DEMO_SECRET=<random-secret-at-least-32-characters>
WEBMCP_GUEST_DEMO_USER_ID=<pre-seeded-viewer-user-id>
WEBMCP_GUEST_DEMO_ORGANIZATION_ID=<pre-seeded-organization-id>
WEBMCP_GUEST_DEMO_BUSINESS_ID=<pre-seeded-business-id>
WEBMCP_GUEST_DEMO_GOAL_ID=<pre-seeded-goal-id>
WEBMCP_GUEST_DEMO_TOKEN_TTL=15m
```

Guest sessions are issued only by `POST /api/v1/guest/session`, kept in browser
memory, expire quickly, and are restricted to one configured business and goal.
Guests may inspect, analyze and create non-canonical demo scenarios and
proposals. Constraint mutation, approval and commit remain forbidden. Use a
dedicated demo organization and never reuse a real user or database secret.

Before enabling this mode, run migrations and seed the referenced viewer user,
organization, business and active goal. The demo business must also contain at
least one active product plus the snapshot data used by the judge flow. The
service readiness check verifies these anchors in the tenant-scoped database
transaction; these IDs are scope anchors, not values that the guest endpoint
creates dynamically. Guest session issuance also refuses to mint a token while
the same readiness check is failing.

For a dedicated judge database, the repository includes an idempotent seed
command. Set the four guest IDs and the target direct (non-pooled) connection
in the local process environment, apply migrations, then run:

```powershell
go run ./cmd/migrate
go run ./cmd/seed-demo
```

The command inserts only missing rows, never overwrites existing rows, verifies
the tenant-scoped anchors in the same transaction, and exits non-zero on a
scope conflict. Review the target project and branch before running it.

## Release checks

1. Deploy the repository with the included `Dockerfile`.
2. Confirm `GET /healthz` returns HTTP 200. This only proves the process is
   alive.
3. Confirm `GET /readyz` returns HTTP 200. This proves authentication and the
   managed PostgreSQL dependency are configured.
4. Configure the frontend `VITE_API_BASE_URL` to the Railway HTTPS service
   URL and rebuild the frontend. If omitted in a production build, the client
   uses same-origin requests rather than silently calling localhost.
5. Run the judge flow with a real authenticated session: snapshot → scenario
   simulation → comparison → proposal review → human approval → commit →
   post-commit verification.

For a standalone real-auth deployment, the host must inject the frontend
`window.__WEBMCP_SESSION__` adapter. It must obtain an OIDC access token from
the identity provider, keep it out of the bundle and browser storage, and
return it only from `getAccessToken()`. A frontend build alone cannot create a
real owner/operator session.

The migration runner is intentionally separate from the web process. Apply
and verify migrations against the target Neon database before the service is
marked ready; do not run migrations on every web instance startup.

For guest judging, use the configured guest session through proposal review;
approval, commit and post-commit verification require a real owner/operator
session because guest mode is intentionally viewer-only.
