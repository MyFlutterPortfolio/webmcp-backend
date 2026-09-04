# Product Priority Model

**Status:** Governing implementation decision

The project is optimized for a top-tier WebMCP Challenge submission. We deliberately concentrate the highest design, engineering, testing and demo effort on WebMCP leverage and the human-agent experience visible to judges.

## Priority tiers

### Tier 1 — Exceptional quality required

- browser-native WebMCP registration and lifecycle;
- semantic, composable tool design;
- precise input/output schemas and structured results;
- real tool discovery and invocation in the judging runtime;
- tool state visibility and safe operational trace;
- human constraint → tool invocation → revised scenario “magic moment”;
- current/proposed/approved/committed state visualization;
- complete, calm, understandable judge experience;
- WebMCP runtime compatibility, fallback and testing instructions;
- challenge demo, public deployment and English submission materials.

The governing detail is [WEBMCP_MASTER_INTEGRATION.md](WEBMCP_MASTER_INTEGRATION.md).

### Tier 2 — Production-safe MVP quality

- backend authorization and tenant boundary;
- deterministic domain calculations;
- proposal/version/approval/commit integrity;
- idempotency, stale-state detection and auditability;
- truthful failures and provider fallback;
- PostgreSQL persistence and transaction correctness;
- API contract, observability and deployability.

Tier 2 may remain narrow in breadth, but it must not be weak in security, correctness or reliability.

### Tier 3 — Explicitly deferred

- broad business modules;
- multi-agent collaboration;
- autonomous background operation;
- external integrations;
- advanced forecasting;
- mobile client;
- microservices, Kubernetes and speculative infrastructure;
- non-critical administrative polish.

## Decision rule

When time or scope conflicts appear, preserve this order:

```text
WebMCP authenticity
→ human-agent collaboration clarity
→ end-to-end demo reliability
→ security and state integrity
→ breadth and optional polish
```

No shortcut may weaken approval protection, tenant isolation, deterministic truth, idempotency or failure safety.
