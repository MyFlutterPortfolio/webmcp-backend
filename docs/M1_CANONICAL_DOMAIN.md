# M1 Canonical Domain & State Machine

M1 establishes the pure domain rules that every future HTTP, database, AI and WebMCP adapter must use.

## Domain packages

- `business`: products, customers, orders, metrics and versioned business snapshot;
- `planning`: human objective, horizon, preference and constraints;
- `scenario`: non-canonical alternatives, assumptions, simulation result and risks;
- `proposal`: versioned recommendation and change set;
- `approval`: human decision bound to one proposal version;
- `commit`: idempotent consequential operation and commit validation policy;
- `audit`: append-only operational event model;
- `core`: shared identity and actor primitives.

## Commit guard sequence

The pure commit policy requires all of the following:

1. valid business snapshot, proposal, approval and operation;
2. authenticated/authorized caller represented by the application layer;
3. proposal belongs to the same business;
4. proposal base version equals the current business version;
5. proposal version is approved;
6. approval matches proposal, version and business exactly;
7. expected business version still matches;
8. operation identity and idempotency key are consistent.

The policy validates; it does not mutate persistence. A later application service will execute the commit inside one database transaction.

## State boundaries

Canonical business data is never represented by scenario or AI output. Scenario and proposal transitions are explicit, invalid transitions are rejected, and audit events have no update API.
