# WebMCP Master Integration Standard

**Status:** Governing WebMCP implementation standard
**Priority:** Tier 1 — exceptional quality required

## 1. Core position

WebMCP is not an integration checkbox, an API mirror, a chatbot shortcut or a collection of UI button wrappers. It is the agent-facing operating surface of the shared business workspace.

```text
Human context
→ Shared workspace
→ Semantic WebMCP capability
→ Application use case
→ Deterministic business logic
→ Structured result
→ Agent interpretation
→ Human judgment
```

Removing WebMCP must materially reduce how the agent can understand, explore and shape the business workspace. If removing a tool changes nothing in the product experience, that tool is not essential enough for the MVP.

## 2. Browser-native boundary

WebMCP runs in the browser tab through the supported `document.modelContext` API. The frontend owns registration and lifecycle; the backend owns truth, authorization and business rules.

- no backend MCP transport;
- no HTTP, SSE or `stdio` implementation pretending to be WebMCP;
- tools call backend application APIs through HTTPS;
- no secrets, database credentials or provider keys in tool code;
- secure context, origin isolation and `tools` Permissions Policy are release requirements;
- unsupported WebMCP degrades to a complete human-only workspace.

## 3. Capability graph, not CRUD list

Tools are organized around the human-agent workflow and semantic business capabilities:

```text
ORIENT
  get_business_snapshot
  get_business_metrics
  get_active_goals

INVESTIGATE
  inspect_business
  analyze_business

EXPLORE
  create_scenario
  simulate_strategy
  compare_scenarios

COLLABORATE
  human constraint editor (UI-owned)
  revise_proposal

REVIEW
  request_human_review

COMMIT
  commit_approved_proposal  [contextually gated]
  verify_committed_result
```

The names are semantic contract candidates, not final code names. Each capability must have an independent reason to exist, precise input schema, structured output, clear state effect and a testable failure contract.

## 4. State-aware exposure

The registry exposes the smallest useful capability surface for the current workspace state. Registration is progressive and lifecycle-bound:

```text
Orient       → read tools
Investigate  → read + analyze tools
Explore      → scenario + simulation tools
Collaborate  → human constraint editor + revision tools
Review       → request-review tool
Approved     → gated commit + verification tools
```

Tools are registered/unregistered with `AbortSignal` when route, workspace, authorization or workflow context changes. The agent must never retain a stale capability surface after a business version or proposal version changes.

## 5. Tool contract standard

Every tool must define:

- stable semantic name and concise positive description;
- exact JSON input schema with typed fields, bounds and meaningful descriptions;
- read/write classification and risk level;
- current workspace/proposal version expectation where relevant;
- cancellation behavior through the execution signal;
- structured success result;
- structured failure result with retryability and safe next step;
- affected entities and resulting workspace revision;
- provenance separating verified business facts from agent assumptions;
- audit metadata without raw sensitive payloads.

Tools accept user-level intent and constraints. They must not ask the agent to perform business arithmetic that the deterministic domain engine can perform reliably.

## 6. Result envelope

The frontend adapter normalizes every execution into a judge-readable result envelope:

```json
{
  "ok": true,
  "tool": "simulate_strategy",
  "operation_id": "op_123",
  "workspace_revision": 7,
  "state": "working",
  "data": {},
  "affected_entities": [],
  "warnings": [],
  "next_actions": ["compare_scenarios"]
}
```

Failure uses the same shape with `ok: false`, a machine-readable error code, truthful message, `retryable`, and a safe recovery action. A failed tool can never produce a success-shaped result.

## 7. Human-agent co-creation contract

The strongest visible WebMCP moment is not “the agent clicked a button”. It is:

```text
Human changes budget/risk/inventory constraint in the UI-owned editor
→ backend persists working constraint revision
→ revise/simulate capability
→ deterministic engine recalculates trade-offs
→ agent receives structured result
→ previous and revised proposal are compared
→ human reviews the changed recommendation
```

The UI must make the tool call, state revision, changed assumptions, metric delta and human control visible without exposing hidden chain-of-thought.

## 8. Consequential commit gate

`commit_approved_proposal` is the highest-risk capability and is never freely available to an agent.

The browser registration and backend must both require:

1. authenticated owner/operator principal;
2. exact approved proposal version;
3. matching current business version;
4. human approval record;
5. separate human confirmation state outside agent-generated text;
6. idempotency key bound to the logical operation;
7. backend authorization and domain revalidation;
8. post-commit verification.

The agent cannot manufacture the confirmation state, approval record or authority token. If any condition is absent, the tool returns a structured `human_confirmation_required`, `stale_proposal`, `approval_required` or authorization error and canonical state remains unchanged.

## 9. Judge-visible evidence

Before release, the following must be demonstrable in the target runtime:

- tool discovery shows semantic names and useful descriptions;
- tool inspector can parse every input schema;
- the agent invokes tools instead of guessing visual controls;
- tool results contain structured business evidence;
- the human constraint changes the next result;
- tools appear/disappear as workflow context changes;
- cancellation removes stale registration and stops in-flight work safely;
- unsupported WebMCP leaves the human workspace usable;
- commit is visibly human-approved and backend-verified.

## 10. Anti-patterns prohibited

- one generic `ask_ai` tool;
- CRUD wrappers exposed as business capabilities;
- a tool whose only effect is clicking a UI control;
- hidden state mutation during a read or simulation;
- agent-supplied approval text treated as authorization;
- client-only permission checks;
- raw LLM numbers treated as business truth;
- exposing hidden chain-of-thought;
- registering every tool permanently regardless of context;
- relying on unsupported WebMCP primitives or undocumented runtime behavior.

## 11. Implementation acceptance gate

No WebMCP tool enters the MVP registry unless it passes semantic review, schema validation, backend authorization review, failure-path tests, lifecycle tests, human-agent demo validation and target-runtime verification.
