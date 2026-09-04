# M9 — Proposal Co-Creation and Human Review

## Delivered workflow

```text
simulated scenario → draft proposal → revision(s) → human review
```

The proposal boundary makes the human-agent collaboration explicit. A scenario contains deterministic modeled outcomes; a proposal packages that scenario into a reviewable decision with a versioned summary and changes.

## API boundaries

### `POST /api/v1/proposals`

Creates a draft proposal from a `simulated` or `compared` scenario. The backend verifies business, goal and scenario identity inside the tenant, generates proposal/version IDs, and records the creation audit event.

### `POST /api/v1/proposals/{proposalId}/revisions`

Accepts bounded feedback and creates a new draft version. The previous version is marked `superseded`; the new version becomes current in one transaction. Revisions cannot mutate an in-review, approved, committed or superseded version.

### `POST /api/v1/proposals/{proposalId}/review`

Requires the active business ID and transitions only the current draft version to `in_review`. It does not approve, authorize or commit the proposal. Approval remains a separate human action and is bound to the exact proposal version in the existing commit contract.

## Safety properties

- proposal creation requires a calculated scenario;
- all proposal mutations require owner/operator authorization;
- every request is tenant-scoped and request-correlated;
- JSON bodies reject unknown fields and trailing values;
- revisions preserve the previous version for comparison and auditability;
- a review request cannot bypass approval or commit validation;
- agent feedback remains working proposal content, never canonical business state.

## WebMCP surface

The native registry now exposes `draft_proposal` after scenario comparison. `revise_proposal` and `request_human_review` send explicit business/proposal context to the backend. The commit capability remains separately gated by human confirmation, exact approval and backend revalidation.

## Remaining release gates

- run PostgreSQL migrations and RLS/concurrency integration tests, including approval races;
- configure a real identity provider;
- complete the visual workspace and target-runtime WebMCP demo.
