# REST API Contract Baseline

**Status:** M10 hardened contract baseline
**Version:** `/api/v1`

## Contract rules

- JSON request/response bodies use explicit schemas;
- every response carries `request_id`;
- errors expose stable machine-readable `code` and safe human-readable `message`;
- organization and resource ownership are enforced server-side;
- consequential requests require `Idempotency-Key`;
- clients never submit approval as free-form agent text;
- provider-specific AI output is never exposed as canonical business state;
- long-running work returns a workflow resource rather than blocking an HTTP request indefinitely.

## Resource surface

| Resource | Purpose | Mutation class |
|---|---|---|
| `/api/v1/businesses/{business_id}/snapshot` | current business state and metrics | read |
| `/api/v1/agent/chat` | grounded advisory conversation plus optional typed capability suggestion | advisory/read |
| `/api/v1/analysis` | verified snapshot + goal evidence, signals and trade-offs | read/derived |
| `/api/v1/goals` | human planning intent | human write |
| `/api/v1/goals/{goal_id}/constraints` | explicit human constraints | human write |
| `/api/v1/workspaces/{workspace_id}/runs` | agent workflow lifecycle | controlled write |
| `/api/v1/scenarios` | alternatives and simulation results | working state |
| `/api/v1/proposals` | reviewable recommendations and versions | reviewable state |
| `/api/v1/proposals/{proposal_id}/approval` | human approval/rejection | human write |
| `/api/v1/proposals/{proposal_id}/commit` | approved consequential commit | highest risk |
| `/api/v1/activity` | safe operational trace and audit read model | read |

Exact request and response schemas are maintained in [openapi.yaml](openapi.yaml) and must remain consistent with WebMCP tool schemas.

The implemented snapshot and commit handlers are documented in [M6_API_INTEGRATION.md](M6_API_INTEGRATION.md). The API remains fail-closed when no real authenticator is configured.

## Agent chat contract

`POST /api/v1/agent/chat` reads the authorized snapshot and goal, projects a bounded non-PII context to the configured Gemini adapter, and returns a concise response. If the next step is unambiguous, the response may include one `tool_suggestion` using a semantic WebMCP name and typed arguments. The frontend presents that suggestion as an explicit human action; the chat route never executes tools, approves proposals or commits business state.

When `WEBMCP_GEMINI_API_KEY` is absent or the provider is unavailable, the same contract is served by the deterministic safe guide. This keeps the judge path functional while preserving the provider boundary. Conversation history is client-held and bounded; raw prompts and provider payloads are not logged.
