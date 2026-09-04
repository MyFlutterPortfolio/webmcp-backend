# WebMCP Approved Architecture Map

**Status:** Approved general architecture baseline
**Source:** `C:\Users\user\OneDrive\Documents\[8292026 1245 PM] Islombek Abdurasu.txt`
**Scope:** Documents 02–08; UI/UX Design System is the next input and is not included yet.

## Architecture layers

| Document | Confirmed responsibility | Project location |
|---|---|---|
| 02 Experience & Core Workflow | Seven-stage human-agent workflow and demo narrative | Frontend + shared product contract |
| 03 WebMCP Capability & Tool Contract | Semantic tools, schemas, risk levels and tool protocol | Frontend adapter + backend validation |
| 04 System Architecture & Domain Model | Modular monolith, domain boundaries and dependency direction | Backend |
| 05 Data Model & Persistence | PostgreSQL ownership, state/versioning, transactions and persistence invariants | Backend |
| 06 Tool Registry & Interaction Protocol | Registry lifecycle, capability discovery, structured results and composition | Frontend WebMCP layer + backend services |
| 07 Security & Threat Model | Trust boundaries, authorization, prompt/tool injection defense and fail-closed rules | Frontend + backend, enforced server-side |
| 08 AI Agent & Co-Creation | Agent lifecycle, context hierarchy, planning, constraints, revision and verification | Backend orchestrator + shared workspace UX |

## UI/UX extension

The UI/UX architecture has now been derived from this approved baseline and is maintained in the separate frontend project: [UI_UX_ARCHITECTURE.md](../../webmcp-frontend/docs/UI_UX_ARCHITECTURE.md). It defines the adaptive decision workspace, premium visual tokens, human-agent interaction model, WebMCP visibility, accessibility, resilience, SEO and performance acceptance gates without changing backend authority or the human approval boundary.

## Canonical runtime model

```text
Human goal / constraints
        ↓
Frontend shared workspace
        ↓
Browser-native WebMCP semantic capability registry
        │ HTTPS API
        ↓
Go modular monolith
  Agent Orchestrator → AI Gateway
        ↓
Application services
        ↓
Deterministic domain / scenario / approval rules
        ↓
PostgreSQL authoritative state
        ↓
Structured result → human review → approval → idempotent commit → verification
```

## Confirmed invariants

- Human remains the decision authority; the agent is untrusted.
- The agent must understand and ground recommendations before acting.
- WebMCP is first-class and semantic, but never an authorization layer.
- WebMCP runs in the browser tab as a client-side tool surface; it is not a backend HTTP/SSE/stdio transport.
- WebMCP capability execution reaches backend application services through HTTPS; the backend remains the authority.
- WebMCP cannot access the database directly or bypass application services.
- AI output is advisory; deterministic calculations and business rules are authoritative.
- Current business state, working state, scenarios, proposals, approvals and committed state are distinct.
- Approval is bound to a specific proposal version and commit validates current business version.
- Consequential operations are idempotent, auditable and fail closed.
- Tool/provider failure cannot create partial mutation or false success.
- Tenant ownership and authorization are enforced by the backend.
- MVP is a small, deep, demonstrable workflow in a modular monolith; future agents, integrations, SSO, advanced autonomy and service extraction are not MVP requirements.

## Implementation interpretation

The architecture is now the governing baseline for implementation. The remaining work is not to redesign these fundamentals, but to derive concrete contracts and code from them in sequence:

1. UI/UX design system and exact screen-to-tool mapping.
2. Final MVP domain and deterministic business dataset.
3. OpenAPI and WebMCP schemas.
4. PostgreSQL migrations and state machines.
5. Backend application services and AI orchestration.
6. Frontend workspace and WebMCP adapter.
7. Security, contract, end-to-end and public-demo verification.

Until the next UI/UX document arrives, no visual layout or screen-specific implementation should be treated as final.
