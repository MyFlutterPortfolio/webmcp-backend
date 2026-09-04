# M8 — Scenario Persistence and Workflow API

## Delivered vertical slice

The scenario capability graph now has a backend path for the core exploration loop:

```text
create_scenario → run_scenario → compare_scenarios → human review
```

The browser remains the WebMCP runtime. The Go backend owns authentication, tenant scope, snapshot reads, scenario persistence and deterministic simulation.

## API boundaries

### `POST /api/v1/scenarios`

Creates a draft working scenario from the current business version. The request carries typed actions and the server derives the scenario ID and base version. The database verifies that the business, goal and authenticated actor belong to the same organization.

### `POST /api/v1/scenarios/{scenarioId}/simulation`

Requires `business_id` in the request body. The service reads the authoritative snapshot, goal and draft scenario, runs the deterministic engine, then persists only if the scenario version and base business version still match. A successful simulation advances the working scenario to `simulated` and writes an audit event.

### `POST /api/v1/scenarios/compare`

Accepts two to eight unique scenario IDs from one business and one base version. It returns projected metrics, deltas, warnings and risks. Automatic recommendation is withheld when every candidate has a high-severity risk. Comparison does not mutate canonical business state.

## Safety properties

- all routes are behind the fail-closed authenticator middleware;
- viewer principals cannot create or simulate scenarios;
- unknown JSON fields and trailing JSON values are rejected;
- scenario actions are bounded by domain validation;
- tenant scope is established with `SET LOCAL app.organization_id`;
- stale simulations fail with a conflict rather than overwriting a newer revision or persisting results against a newer business version;
- scenario creation and simulation require an owner/operator at the database boundary and are request-audited;
- database writes are parameterized and audit-correlated;
- LLM output is not accepted as executable action or business truth.

## Remaining release gates

- run the migration against PostgreSQL and test RLS with two organizations;
- select and implement the real JWT/session authenticator;
- verify the live WebMCP tool discovery and invocation path in the target browser;
- complete the judge-facing workspace UX and one public end-to-end demo.
