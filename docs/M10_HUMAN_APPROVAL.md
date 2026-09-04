# M10 — Human Approval Engine

## Decision boundary

`POST /api/v1/proposals/{proposalId}/approval` is the only approval write path. It requires an authenticated owner or operator and accepts only `approved` or `rejected` with a non-empty reason. No WebMCP tool exposes approval, so an agent cannot manufacture human authority.

## Transactional guarantees

The PostgreSQL adapter executes the following under one tenant-scoped transaction:

1. lock the authoritative business row and read its current state version;
2. verify the actor belongs to the tenant and has owner/operator authority;
3. lock the exact current proposal version and require `in_review`;
4. require the proposal base business version to equal the current business version;
5. persist the decision, transition the proposal version, update proposal freshness and append an audit event.

Any failure rolls back the complete decision. A stale proposal, already-decided version, cross-tenant identity or unauthorized actor cannot partially change state.

## Commit relationship

Approval is necessary but not sufficient for commit. The existing commit transaction independently revalidates the approved version, approval actor, current business version, authorization and idempotency before changing canonical business state.

## Verification boundary

Application and HTTP contract tests cover decision validation, role protection and response shape. Live PostgreSQL migration, RLS and concurrent approval tests remain release gates until a database environment is available.
