# WebMCP Implementation Roadmap

**Status:** Active execution plan
**Quality bar:** Top-tier hackathon product with production-grade boundaries, security and verification.

## Delivery order

1. **Foundation** — repository conventions, Go module, configuration, HTTP server lifecycle, health/readiness, structured errors, correlation IDs and CI checks.
2. **Canonical domain** — business snapshot, planning goal, constraints, scenarios, proposals, approvals, commit operation and explicit state machines.
3. **Persistence** — PostgreSQL migrations, repositories, transaction boundaries, version checks, idempotency and append-only audit events.
4. **AI and orchestration** — provider interface, Groq adapter, three-model fallback policy, structured output validation, bounded agent runs and safe tool planning.
5. **WebMCP contract** — semantic tool schemas, registry, capability lifecycle, structured tool results and backend authorization revalidation.
6. **API integration** — authenticated snapshot/commit boundaries, explicit DTOs, idempotent commit payloads and contract tests.
7. **Frontend workspace** — UI/UX document-driven screens, server state, activity trace, scenario comparison, constraint editing, approval and commit UX.
8. **Integration and demo** — one complete workflow, failure recovery, security tests, production builds, deployment smoke checks and judge path.

## Non-negotiable quality gates

- No AI output directly mutates canonical state.
- Every consequential commit validates authorization, proposal version, approval, business version and idempotency.
- Failed, stale or duplicated operations are safe and observable.
- Domain logic is deterministic and testable without an LLM.
- WebMCP remains semantic, isolated and feature-detectable.
- Secrets never enter frontend bundles, prompts, logs or API responses.
- Each milestone must pass focused tests before the next dependent layer is added.

## Current milestone

**M10 — Human approval and backend hardening: implemented.** Owner/operator decisions are bound to the exact current proposal version and business version. Approval/rejection, proposal transition and audit append execute atomically inside a tenant-scoped transaction with row locks. Database writes now revalidate operator roles, stale scenario simulation is rejected, RLS is forced for table owners, production database transport requires verified TLS, CORS preflight is allowlisted, sensitive API responses are non-cacheable, and readiness checks required dependencies. The browser agent has no approval capability; human authority remains an explicit boundary. Live PostgreSQL/RLS integration and WebMCP verification in ChatGPT’s in-app browser and Chromium testing runtime remain release gates.

Top-3 challenge alignment and live-demo gates: [DEVPOST_TOP3_ALIGNMENT.md](DEVPOST_TOP3_ALIGNMENT.md).
Implementation priority policy: [PRIORITY_MODEL.md](PRIORITY_MODEL.md).


## Verification commands

Backend target: `gofmt`, `go test ./...`, `go vet ./...`, `go build ./...`.

Frontend target after bootstrap: type-check, lint, production build and WebMCP feature-detection tests.

## Validation boundaries

- `go test -race ./...` is pending on the current Windows machine because CGO requires an installed C compiler.
- Docker image smoke build is pending because Docker Desktop/Linux Engine is not running in the current environment.
