# Northstar

## Human + Agent Business Workspace

Northstar is a WebMCP-native workspace for turning business intent into a
verified, human-controlled decision.

It is not another CRM, ERP, admin panel or generic AI chatbot. Northstar gives
an agent a semantic capability surface inside the live application, lets it
inspect evidence, explore bounded alternatives and prepare a proposal, then
keeps approval and consequential execution with a human.

> **Hackathon status:** this repository contains a focused judge-facing demo
> and a production-shaped foundation. The demo uses a bounded seeded business
> dataset; it is not presented as a complete multi-tenant SaaS product.

## Live demo

- [Open the Northstar workspace](https://webmcp-frontend-production.up.railway.app)
- [Backend health](https://webmcp-backend-production.up.railway.app/healthz)
- [Backend readiness](https://webmcp-backend-production.up.railway.app/readyz)
- [Frontend repository](https://github.com/MyFlutterPortfolio/webmcp-frontend)
- [Backend repository](https://github.com/MyFlutterPortfolio/webmcp-backend)

The guest entry is designed for a fast judge experience. It is deliberately
scoped to one seeded business and one goal, and cannot approve or commit a
decision. The full commit path requires a real owner/operator session supplied
by a trusted host.

## The problem

Business decisions are scattered across dashboards, spreadsheets, analytics,
messages and disconnected AI chats. A conversational model may produce a
persuasive answer, but an answer is not a decision workflow:

- it may not be grounded in the current business state;
- assumptions and trade-offs are difficult to review;
- a proposed action can be confused with a real action;
- the human decision boundary is often invisible;
- a failed request can be mistaken for success.

Northstar turns that fragmented loop into one visible workspace where evidence,
assumptions, proposals, human feedback, approval and verification have clear
state boundaries.

## The product idea

The workspace follows a seven-stage decision journey:

~~~text
Orient
  -> Define
  -> Investigate
  -> Explore
  -> Collaborate
  -> Approve
  -> Commit
~~~

The focused demo tells one complete story: a founder or operator wants to grow
repeat revenue safely over 90 days. Northstar reads a verified business pulse,
analyzes the opportunity, creates and simulates alternatives, compares their
trade-offs, prepares a proposal, incorporates a human constraint, and reaches
the commit gate only after explicit approval.

## Why WebMCP is the core of Northstar

WebMCP is not used here as a checkbox, a hidden HTTP endpoint or a button
wrapper. It is the agent-facing operating surface of the product.

Without WebMCP, the agent can only describe what a human might click. With
Northstar's browser-native capability layer, the agent can discover what the
current workspace allows, call a typed business capability and receive a
structured result that changes the next step of the collaboration.

The important interaction is:

~~~text
Human intent and constraints
  -> stage-aware WebMCP capability surface
  -> verified backend use case
  -> deterministic business calculation
  -> structured result
  -> agent interpretation
  -> human judgment
~~~

This is the difference between an AI assistant that talks about a business and
an agent that can work with a business application while remaining bounded by
human authority.

### The WebMCP magic moment

1. The browser registers only the capabilities relevant to the current journey
   stage.
2. The agent discovers semantic tools such as
   get_business_snapshot, analyze_business and create_scenario.
3. The tool executes through the frontend WebMCP registry and HTTPS backend
   API; it does not click UI controls or access the database directly.
4. The backend authenticates, authorizes, validates and applies the use case.
5. The deterministic scenario engine returns evidence, modeled deltas,
   assumptions, warnings and next actions in a structured envelope.
6. The human changes a meaningful constraint in the workspace.
7. The agent can use the revised capability surface and the next simulation
   visibly changes.

That human constraint changing the recommendation is the proof of real
co-creation. The agent is not performing a scripted dashboard tour.

### Semantic capability graph

| Capability | Agent role | State boundary |
|---|---|---|
| get_business_snapshot | Read the current business pulse | Authoritative read |
| analyze_business | Find evidence, signals and trade-offs | Derived read |
| create_scenario | Shape a bounded alternative | Working state |
| run_scenario | Simulate a strategy | Non-canonical |
| compare_scenarios | Compare modeled alternatives | Non-canonical read |
| draft_proposal | Turn a simulation into a reviewable proposal | Working state |
| revise_proposal | Apply human feedback to a proposal version | Working state |
| request_human_review | Ask for explicit review | Human gate |
| commit_approved_proposal | Execute the exact approved version | Consequential, gated |
| verify_committed_result | Read back the authoritative result | Verified read |

Tools are progressively exposed by stage and removed through AbortSignal
when the stage, session or workspace context changes. Each tool has a precise
JSON schema, risk annotation, bounded inputs, cancellation behavior, structured
success result and structured failure result.

The chat is intentionally secondary to the capability graph. Northstar can
recommend one typed capability, but chat never executes tools, creates an
approval record or suggests a free-form commit.

## Human authority and trust model

Northstar separates four kinds of state:

~~~text
Current business state   = authoritative PostgreSQL state
Scenario / proposal      = working, non-canonical state
Approval                 = explicit human decision on one exact version
Commit                   = backend-gated consequential transition
~~~

The commit boundary requires all of the following:

- authenticated owner/operator authority;
- the exact approved proposal version;
- the current business version still matching the proposal;
- an approval record with an auditable reason;
- a separate human confirmation outside agent-generated text;
- backend authorization and domain revalidation;
- an idempotency key for replay safety;
- post-commit read-back verification.

If transport status is uncertain, the UI shows an unknown state and reconciles
against the authoritative snapshot. It never invents a successful commit.

## Architecture

~~~text
React + TypeScript + Vite + Tailwind
        |
        | HTTPS / typed REST
        v
Go application backend
  |-- auth and tenant policy
  |-- WebMCP use-case boundary
  |-- deterministic analysis and scenario engine
  |-- proposal and approval workflow
  |-- idempotent commit and verification
  |-- Groq advisory router
        |
        +--> Neon PostgreSQL
        |      authoritative business and workflow state
        |
        +--> Groq Chat Completions
               primary key -> fallback key 1 -> fallback key 2
               -> deterministic safety guide
~~~

### Backend

- Go modular monolith with explicit application, domain, persistence and HTTP
  boundaries.
- PostgreSQL is the business source of truth; migrations are ordered and the
  current deployment uses Neon with verified TLS.
- Server-side tenant scope, authorization, request validation, rate limiting,
  correlation IDs and safe error envelopes.
- Deterministic calculations stay outside the LLM, so generated numbers never
  become business truth.
- Canonical state, working state, approval and commit transitions are kept
  distinct and auditable.

### AI routing

Groq is advisory infrastructure, not an authority layer. The deployed router
uses three isolated Groq key slots. Each slot can use its own model ID; the
current demo uses openai/gpt-oss-120b for all three slots, followed by the
deterministic guide when external inference is unavailable.

Every provider response is bounded, parsed as a JSON object and validated
against the application contract. The backend forwards a compact non-PII
projection of the authorized workspace rather than raw customer records,
orders or database-shaped payloads. Groq keys stay in Railway server secrets;
they never enter the frontend bundle, WebMCP tools, logs or API responses.

### Frontend and WebMCP runtime

The frontend owns the browser-native document.modelContext lifecycle,
registration, schema exposure and cancellation. It is feature-detectable: a
normal browser keeps the human-only workspace usable when native WebMCP is not
available.

For the target Chromium runtime, the app verifies secure context, origin
isolation, tools Permissions Policy, registration, discovery, execution and
abort cleanup. The native runtime is experimental, so the target-browser check
must be repeated immediately before submission.

## What is implemented in this demo

- Premium adaptive human-agent workspace rather than a conventional admin
  panel.
- Live guest entry with a short-lived, in-memory, fixed business scope.
- Verified business pulse and goal grounding.
- Agent chat with Groq routing, two keyed fallbacks and deterministic fallback.
- Stage-aware WebMCP registry with semantic typed capabilities.
- Deterministic business analysis and scenario simulation.
- Scenario comparison with recommendation basis and trade-offs.
- Proposal creation, versioning, human feedback and proposal diff.
- Human approval reason, approval state and commit confirmation gate.
- Idempotent backend commit and post-commit verification UX.
- Truthful cancellation, stale state, network uncertainty and provider fallback
  states.
- Docker and Railway deployment shape with health and readiness probes.

## Deliberate demo boundaries

The hackathon release is intentionally focused so the WebMCP interaction is
visible and judgeable:

- data is a seeded demonstration business, not a customer production account;
- guest mode is viewer-oriented and cannot approve or commit;
- real owner/operator authentication is supplied by a trusted host through the
  provider-neutral OIDC/JWKS boundary;
- business calculations are deterministic and bounded rather than a claim of
  general-purpose forecasting;
- external commerce, finance, CRM and ERP connectors are future product work;
- the frontend does not persist bearer tokens in local storage or Vite files;
- native WebMCP support depends on the target browser runtime.

These constraints are part of the trust story, not missing safeguards.

## Why this can become a startup

Northstar starts with a narrow, expensive problem: helping a founder or
operations team make a constrained growth decision with evidence and a clear
path to action.

### Initial wedge

The first customer is a small or mid-sized digital business that already has
data in several systems but lacks a reliable decision workflow. The product
value is measured in decision quality and time-to-action, not in another
dashboard page:

- less time assembling evidence;
- explicit assumptions and trade-offs;
- lower risk of accidental or unauthorized change;
- a decision record that can be reviewed later;
- a repeatable operating rhythm for 30/60/90-day goals.

### Expansion path

1. **Connected decision workspace** — Shopify, Stripe, HubSpot, analytics and
   inventory connectors with normalized evidence.
2. **Industry decision packs** — retention, pricing, inventory, marketing,
   hiring and cash-flow workflows with domain-specific deterministic models.
3. **Enterprise governance** — role-based approvals, policy packs, audit export,
   SSO, data residency, retention controls and environment separation.
4. **Agent capability platform** — a versioned WebMCP capability SDK, partner
   integrations and reusable human-approval patterns.
5. **Learning loop** — compare modeled outcomes with verified results and use the
   evidence to improve assumptions without allowing an LLM to become the
   source of truth.

### Sustainable business model

- workspace subscription for growing teams;
- usage-based connected data and advanced simulation tiers;
- enterprise governance, SSO, retention and audit packages;
- partner and embedded-workspace licensing for vertical software platforms.

The long-term moat is not a single model. It is the combination of a trusted
decision state machine, domain evidence, approval history and a browser-native
agent capability layer that can work across applications without giving up
human control.

## Hackathon alignment

| Challenge criterion | Northstar proof |
|---|---|
| WebMCP leverage | Real browser-native discovery, typed semantic tools, structured results, progressive exposure and execution effects in the workspace. |
| Execution | A runnable live flow from business pulse to scenario, proposal, human gate and verified outcome. |
| Potential impact | A concrete 90-day growth decision for founders and operators, with evidence and trade-offs in one place. |
| Creativity and ambition | Business co-creation where the agent explores and proposes while the human changes constraints and retains authority over commitment. |

The judge-facing narrative is intentionally short:

~~~text
Open workspace
  -> inspect verified pulse
  -> ask Northstar a business question
  -> run a typed WebMCP capability
  -> create, simulate and compare alternatives
  -> change a human constraint
  -> revise the proposal
  -> review and approve
  -> commit only with separate human confirmation
  -> verify the authoritative result
~~~

## Quick start

The frontend and backend are intentionally separate production projects.

### Backend

~~~powershell
git clone https://github.com/MyFlutterPortfolio/webmcp-backend.git
cd webmcp-backend
go mod download
~~~

Set development environment variables in the shell or a local secret manager.
Use .env.example as the variable reference. Never commit a real database or
Groq credential.

~~~powershell
go run ./cmd/migrate
go run ./cmd/seed-demo
go run ./cmd/server
~~~

The local service defaults to http://127.0.0.1:8080. See the deployment and
Neon documents for production database setup, guest anchors and Railway
configuration.

### Frontend

~~~powershell
git clone https://github.com/MyFlutterPortfolio/webmcp-frontend.git
cd webmcp-frontend
npm ci
$env:VITE_API_BASE_URL="http://127.0.0.1:8080"
npm run dev
~~~

VITE_API_BASE_URL is a public endpoint configuration, not a secret. In
production it must be the exact HTTPS backend origin.

### Target WebMCP runtime

For the current Chromium testing path:

1. Open chrome://flags/#enable-webmcp-testing and enable the WebMCP testing
   flag.
2. Relaunch Chrome and open the HTTPS demo.
3. Run **Run runtime check** in the Agent panel.
4. Confirm registration, discovery, execution and abort cleanup.

A regular browser without native WebMCP should still present the human-only
workspace and explain the unavailable capability surface.

## Verification

Backend checks:

~~~powershell
go test ./...
go vet ./...
go build ./...
~~~

Frontend checks:

~~~powershell
npm run typecheck
npm run test
npm run build
~~~

The release gate also includes live /healthz and /readyz checks, guest session
scope validation, real Groq chat verification, frontend security headers and
target-browser WebMCP runtime QA.

## Documentation

- [Architecture](docs/ARCHITECTURE.md)
- [REST API contract](docs/API_CONTRACT.md)
- [OpenAPI specification](docs/openapi.yaml)
- [WebMCP integration standard](docs/WEBMCP_MASTER_INTEGRATION.md)
- [Judge demo runbook](docs/M15_RAILWAY_DEPLOYMENT.md)
- [Challenge alignment](docs/DEVPOST_TOP3_ALIGNMENT.md)
- [Neon PostgreSQL integration](docs/M14_NEON_MANAGED_POSTGRES.md)

## License

Northstar is released under the [MIT License](LICENSE).
