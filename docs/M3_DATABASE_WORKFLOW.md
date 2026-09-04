# M3 Database-Backed Workflow

M3 connects the canonical domain guards to PostgreSQL persistence.

## Transaction sequence

```text
SET LOCAL tenant
→ lock idempotency record
→ create operation reservation
→ lock business version
→ load current proposal version
→ load matching human approval
→ run domain commit policy
→ persist committed decision artifact
→ increment business version
→ mark proposal committed
→ mark operation succeeded
→ append audit event
→ COMMIT
```

Any error rolls back the transaction, so a failed proposal, stale version, missing approval, unauthorized request or duplicate operation cannot partially mutate canonical state.

## Persistence boundaries

- `BusinessRepository.GetSnapshot` reads a tenant-scoped, versioned snapshot.
- `CommitService` owns the consequential transaction and uses domain validation before mutation.
- `WithTenantTx` and `WithTenantTxResult` set `SET LOCAL app.organization_id` for every transaction.
- Idempotency is scoped to organization and locks the existing operation before replay/conflict handling.
- Commit replay returns the previously succeeded outcome; an in-progress replay is rejected rather than duplicated.

## Known verification boundary

The SQL and transaction behavior require a running PostgreSQL instance with migration `000001_initial.up.sql`. The current environment has no PostgreSQL server, so live migration, RLS and concurrent commit integration tests remain explicit next validation work.
