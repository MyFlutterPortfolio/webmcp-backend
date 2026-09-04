# WebMCP Backend Architecture

**Status:** Approved architecture baseline with implemented deterministic MVP boundaries. The sections marked as future extensions are intentionally not part of the current judge release.

## 1. Tizim konteksti

```text
webmcp-client
      │ HTTPS / REST
      ▼
┌──────────────────────────────────────────────┐
│ Go Backend                                   │
│                                              │
│ REST API                                     │
│ Typed WebMCP use-case boundary               │
│ Deterministic workflow services              │
│ Scenario Engine                              │
│ Approval Engine                               │
│                                              │
│ Auth, policy, audit, observability            │
└───────────────┬──────────────────┬───────────┘
                │                  │
                ▼                  ▼
          PostgreSQL           Future AI extension
          Business State       Provider adapter boundary
```

Backend server-side authority hisoblanadi: client’dagi registry yoki UI state business state’ni yakuniy deb hisoblamaydi.

## 2. Responsibility boundaries

### REST API

Transport, authentication, authorization, request validation, rate limiting, response envelope, versioning va correlation ID bilan ishlaydi. Handler’lar business logic joyi bo‘lmaydi.

### WebMCP workflow boundary

Current MVP’da browser agenti `document.modelContext` orqali semantic tool’larni chaqiradi. Backend bu chaqiriqlarni typed application use-case’lariga map qiladi, context va tenant scope’ni server-side tekshiradi hamda state transition’larni persist qiladi. Alohida autonomous planner/orchestrator keyingi extension sifatida ajratilgan.

Agent yoki keyingi planner’dan kelgan hech qanday text to‘g‘ridan-to‘g‘ri command sifatida bajarilmaydi; har bir action policy, schema va state transition’dan o‘tadi.

### Future AI Router boundary

Current judge MVP ushbu komponentga runtime dependency qilmaydi: scenario va business calculations deterministic domain engine’da bajariladi. Kelajakda Gemini primary va fallback provider adapterlari qo‘shilsa, ular typed output validation, timeout, retry budget, circuit breaker, cost metadata va redacted logging boundary’si orqali ulanadi; provider output canonical business state bo‘la olmaydi.

### Scenario Engine

Deterministic business workflow’larni definition, validation, state transition va compensation qoidalari bilan bajaradi. Scenario’lar AI’dan mustaqil bo‘lishi kerak; AI faqat ruxsat etilgan decision point’larda yordam beradi.

### Approval Engine

Riskli yoki tashqi ta’sirga ega action’lar uchun policy evaluation, approver scope, expiry, decision, reason va audit trail’ni boshqaradi. Approval client UX’da ko‘rinadi, lekin qarorning authority’si backend’da qoladi.

### Persistence

PostgreSQL business state, workflow state, approval state, tool/scenario metadata va audit record’lar uchun source of truth bo‘ladi. Migration versioning, transaction boundary va optimistic/concurrency control oldindan belgilanadi.

## 3. Logical module map

```text
transport/http
  └── application services
        ├── WebMCP use-case boundary
        ├── scenarios
        ├── approvals
        ├── tools/policy
        ├── future ai/provider routing
        └── persistence repositories
              └── PostgreSQL
```

Har bir module public contract orqali bog‘lanadi. HTTP modeli, domain modeli va database modeli bir xil struct sifatida ko‘paytirilmaydi.

## 4. Request va execution oqimi

1. Client HTTPS request yuboradi.
2. API middleware request ID, auth, authorization, rate limit va input size’ni tekshiradi.
3. Application service current user/tenant context va idempotency key bilan workflow yaratadi yoki davom ettiradi.
4. WebMCP use-case boundary scenario va tool policy’ni tekshiradi.
5. Deterministic scenario engine typed action’larni hisoblaydi; future AI bo‘lsa, u faqat advisory typed decision qaytaradi.
6. Approval kerak bo‘lsa, workflow `waiting_approval` holatida persist qilinadi.
7. Approval berilgach, action schema, permission va state version bilan qayta tekshiriladi.
8. Tool/action execution timeout va cancellation bilan bajariladi.
9. Natija, transition va audit transaction chegarasida saqlanadi.
10. Client’ga safe response envelope qaytariladi.

## 5. State machine baseline

```text
created → planning → waiting_approval → executing → succeeded
                    └──────────────────────────→ rejected
                         executing → failed → retryable/terminal
```

Allowed transition’lar explicit bo‘ladi. Duplicate request, retry va concurrent approval idempotent hamda version-check bilan himoyalanadi.

## 6. Security baseline

- TLS termination, strict CORS va security headers.
- Authentication va resource-level authorization.
- Least privilege service credentials; LLM/API secretlar faqat secret manager yoki protected runtime configuration’da.
- Input validation, output validation, request size limit, rate limit va abuse protection.
- SQL parameterization, transaction discipline va migration review.
- SSRF, command injection, prompt injection va tool-confusion threat model’i.
- Tool allowlist, capability scope, approval gate va server-side revalidation.
- PII minimization, encryption at rest/in transit, retention policy va audit access control.
- Structured logs’da token, password, API key va sensitive payload redaction.

## 7. API contract baseline

API versioned bo‘ladi. Har bir response’da correlation/request ID va machine-readable error code bo‘ladi. Long-running workflow uchun async resource status modeli ko‘zda tutiladi; polling va streaming variantlaridan biri contract bosqichida tanlanadi.

Expected resource categories:

- sessions/agents;
- tools va tool capabilities;
- scenarios va executions;
- approvals;
- activity/audit read models;
- health/readiness va operational endpoints.

## 8. Reliability va operatsion talablar

- Context-aware timeout va cancellation propagation.
- Faqat xavfsiz/idempotent operation’larda bounded retry.
- Future provider fallback faqat retry storm va duplicate side effect keltirmaydigan policy bilan.
- DB connection pool, transaction timeout va migration compatibility.
- Health, readiness, dependency health va graceful shutdown.
- Metrics: request latency/error, workflow transitions, approval latency, provider usage/cost, fallback rate va tool failures.
- Distributed tracing uchun correlation/trace context.

## 9. Deployment yo‘nalishi

Independent build/test/deploy pipeline. Environment’lar development, staging va production sifatida ajratiladi. Containerized runtime, immutable artifact, migration step, rollback strategiyasi, secret injection va minimal runtime permission deployment design’ning majburiy qismlari hisoblanadi.

## 10. Verification strategy

- `go test ./...`, `go vet ./...`, `go build ./...`.
- Domain state machine va approval policy unit testlari.
- API contract/integration testlari.
- PostgreSQL migration va transaction testlari.
- Future AI provider adapter contract testlari; live provider testlari alohida controlled environment’da.
- Security tests: authorization matrix, input abuse, prompt/tool injection va secret redaction.
- Container smoke test, readiness test va production-like staging verification.

## 11. Current boundary decisions

- Authentication uses the provider-neutral OIDC JWT/JWKS contract documented in [M13_AUTH_SESSION_BOUNDARY.md](M13_AUTH_SESSION_BOUNDARY.md). Provider claim mapping remains a deployment configuration concern.
- The frontend session boundary is an in-memory host adapter; no browser storage or client-side token decoding is allowed.
- Managed PostgreSQL runs on Neon through a direct TLS connection for the current `pgxpool` deployment; schema changes use the ordered migration runner in [M14_NEON_MANAGED_POSTGRES.md](M14_NEON_MANAGED_POSTGRES.md).

## 12. Pending decisions

- Product domain, user roles va tenancy modeli.
- Concrete authentication provider and token/session lifecycle integration.
- Gemini model/version, fallback provider va provider selection policy.
- WebMCP tool execution localmi yoki backend-mediatedmi.
- Queue/worker kerakligi va long-running execution modeli.
- Approval actorlari, risk matrix va emergency/revoke qoidalari.
- PostgreSQL schema, retention, backup/PITR va migration tool.
- Deployment cloud, container platform, domain/TLS va observability stack.
- API specification formati (OpenAPI va schema ownership).
