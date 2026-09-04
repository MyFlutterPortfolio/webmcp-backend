# M2 PostgreSQL Persistence

M2 adds the canonical PostgreSQL persistence boundary without coupling domain rules to SQL.

## Guarantees

- tenant-owned tables contain `organization_id`;
- PostgreSQL Row-Level Security denies access without a transaction tenant context;
- `SET LOCAL` prevents tenant context leakage across pooled connections;
- all money uses integer cents with database checks;
- aggregate versions are positive and proposal base versions are persisted;
- proposal-version approval is explicit;
- commit idempotency is unique per organization;
- committed decision artifacts are separate from scenarios/proposals;
- audit events are append-oriented and have no update path in the application model;
- indexes target business snapshots, workflow status, approvals, commits and audit reads;
- schema changes are forward/reverse migration files.

## Transaction rule

Every tenant operation must use `postgres.WithTenantTx`. Consequential commit will later execute under a single transaction with row locks, proposal/approval/version checks, idempotency lookup, canonical decision insert, business version increment, proposal status update and audit append.

## Deliberate boundary

The migration stores an approved strategy as a `committed_decisions` artifact. It does not invent product/inventory/revenue mutations before the exact business domain change set is approved. Those mutations will be added as domain-specific application commands, not as arbitrary JSON or direct agent writes.
