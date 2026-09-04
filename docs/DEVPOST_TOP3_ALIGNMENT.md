# WebMCP Challenge — Top-3 Alignment

**Status:** Active product and delivery constraint
**Verified:** 29 August 2026

## Official judging model

The challenge has a Stage One viability gate: the project must fit the theme and reasonably use the required APIs/SDKs. Projects that pass are scored equally across four criteria:

1. WebMCP Leverage — genuine, non-trivial and working WebMCP use;
2. Execution — complete, coherent and runnable product experience;
3. Potential Impact — credible solution for a specific real audience/problem;
4. Creativity & Ambition — novel concept and meaningful differentiation.

This changes our delivery priority: a technically impressive backend that is invisible in the live human-agent workflow is not sufficient. Every backend feature must produce a visible, judge-comprehensible collaboration moment.

## Product proof matrix

| Criterion | Product proof | Engineering implication |
|---|---|---|
| WebMCP Leverage | Agent discovers semantic business tools, receives structured results and changes course after human input | Browser-native registry, real tool execution, no button-wrapper theater |
| Execution | Judge completes one flow from business context to committed decision | Seeded deterministic dataset, resilient states, graceful errors and public live URL |
| Potential Impact | Founder/operator makes a better constrained growth decision with less fragmented work | One specific 90-day growth workflow and measurable trade-offs |
| Creativity & Ambition | Human judgment and agent reasoning visibly co-create an outcome | Shared workspace, constraint-driven revision, scenario comparison and approval |

## Non-negotiable demo path

```text
Open live workspace
→ See business state
→ Define 90-day goal
→ Agent invokes browser WebMCP tools
→ Inspect and analyze seeded data
→ Generate and simulate alternatives
→ Human changes a meaningful constraint
→ Agent invokes tools again and revises
→ Human reviews trade-offs
→ Human approves
→ Backend performs idempotent commit
→ Workspace verifies committed result
```

## Submission reliability rules

- Provide a working public live app and public repository with an open-source license.
- Provide clear testing instructions and an English submission/demo path.
- Provide a sub-three-minute demo video with audio explaining the product and WebMCP usage.
- Test the live app in ChatGPT’s in-app browser and Chrome WebMCP runtime before submission.
- After the deadline, do not modify the submitted app, repository or Devpost materials; continue on a separate fork/branch if needed.

## Top-3 quality gates

### Gate A — WebMCP proof

The browser tool inspector must show semantic tools with precise schemas, structured output and real application effects. The demo must make it obvious that the agent used capabilities rather than visually guessing UI controls.

### Gate B — Human agency proof

The human constraint must materially change the subsequent scenario, expected effects or recommendation. A chat message that does not alter structured state does not pass this gate.

### Gate C — Truth and safety proof

The UI must distinguish current, proposed, approved and committed state. The backend must reject stale, unauthorized, replayed or unapproved commits.

### Gate D — Failure proof

Tool errors, provider fallback, unsupported WebMCP and network uncertainty must show truthful, recoverable states. No invented success and no partial canonical mutation.

### Gate E — Live judge proof

The first-run experience must not require a team member to explain the product. Seed data, loading states, empty/error states and testing instructions must be prepared for an unfamiliar judge.

## Runtime facts incorporated

WebMCP is currently an experimental, browser-native API. The target Chromium workflow uses the WebMCP testing flag or origin trial, with imperative or declarative registration. It is gated by secure context, origin isolation and Permissions Policy. The current implementation exposes tools; it is not a backend MCP transport and does not provide Resources or Prompts.

These facts are time-sensitive. Verify the judging runtime against current official documentation before implementation and again before submission.
