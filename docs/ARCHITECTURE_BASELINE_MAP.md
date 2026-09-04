# Architecture Baseline Map

Source: `C:\Users\user\OneDrive\Documents\Arxitekjtura.txt`

**Status:** Non-final baseline. This document is an analyzed integration map; it does not override later approved technical decisions.

## Source document separation

The supplied file contains three logical documents concatenated together:

1. **Project Document 02 — Product Experience & Core Workflow Specification**
   - Defines the seven-stage experience: Orient, Define, Investigate, Explore, Collaborate, Approve, Commit.
   - Defines the judge/demo narrative and the WebMCP “magic moment”.
   - Establishes that conversation is supporting UX, while the artifact/workspace is the product.

2. **WebMCP Architecture / Capability Specification**
   - Defines semantic capability classes: read, analyze, simulate, propose and commit.
   - Requires structured schemas/results, tool validation, progressive/context-aware exposure, cancellation, idempotency and auditability.
   - Treats WebMCP as an isolated agent-facing adapter, never as an authorization or domain-logic bypass.

3. **Technical Architecture Baseline**
   - Defines a modular monolith with React frontend, Go backend, PostgreSQL and AI Gateway.
   - Separates application services, domain logic, persistence and provider adapters.
   - Requires canonical/working state separation, approval and commit boundaries, optimistic concurrency, safe failure and lightweight observability.

## Integrated project model

```text
Human intent and constraints
        ↓
Frontend workspace
        ↓
WebMCP semantic capability adapter
        ↓
Go application services / agent orchestrator
        ↓
Deterministic domain and scenario logic
        ├── PostgreSQL authoritative state
        └── AI Gateway (Gemini primary + fallback)
        ↓
Structured result → human review → approval → idempotent commit
```

## Non-negotiable invariants adopted

- Human approval is required for consequential commitment.
- Agent output is a proposal, never authoritative business state.
- Scenarios and simulations cannot mutate canonical state.
- WebMCP tools are semantic capabilities, not UI wrappers or direct database access.
- Backend independently validates authentication, authorization, schema, business rules, approval, current version and idempotency.
- Deterministic calculations remain outside the LLM.
- Stale proposals, failed tools and provider failures must not silently mutate state or report false success.
- MVP remains one polished end-to-end growth workflow in a modular monolith; no speculative microservices, Kubernetes, Redis or message broker.

## Intentional non-final areas

The following remain open until the next documents or explicit approval:

- exact business domain and deterministic model;
- authentication and user/role model;
- final WebMCP runtime API and judging-browser compatibility;
- exact tool schemas, tool availability lifecycle and execution ownership;
- API/OpenAPI contracts and frontend state implementation;
- database schema and migration strategy;
- AI model versions, fallback provider and retry policy;
- hosting, domain, secrets, observability and demo data;
- whether long-running operations need asynchronous processing.

## Implementation gate

No production feature code should begin from this baseline alone. The next supplied architecture documents become inputs for refining this map, after which the final contract, threat model, state machines and delivery milestones will be approved.
