# M7 — Deterministic Scenario Engine

## Objective

Make the core WebMCP co-creation moment verifiable: an agent proposes a bounded strategic alternative, the system simulates it against an authoritative snapshot, and the human receives explicit trade-offs before any proposal can reach approval.

## Typed action contract

Scenario actions are structured values, not executable prose:

| Type | Unit | Target | Bounds |
|---|---|---|---|
| `price_adjustment` | `percent` | active product | -90% to +200% |
| `inventory_replenishment` | `units` | active product | 1 to 1,000,000,000 whole units |
| `marketing_budget` | `cents` | none | 1 to 1,000,000,000,000 whole cents |

The frontend WebMCP schema and backend domain validator both enforce this boundary. The backend does not treat agent text as an executable instruction.

## Simulation invariants

- candidate scenario must be `draft`;
- scenario business, goal and base version must match the verified snapshot;
- canonical snapshot is immutable during simulation;
- calculations are deterministic and have no LLM or network dependency;
- output contains baseline metrics, projected metrics and deltas;
- hard budget violations become explicit high-severity risks and warnings;
- simulation advances the scenario to `simulated` with a new working revision;
- no simulation output authorizes approval or commit.

The current model is intentionally transparent. Marketing uses a documented fixed return assumption and exposes the resulting spend and margin impact; it is a planning estimate, not a claim about realized revenue.

## Why this matters for WebMCP

The tool surface now carries business intent at the right semantic level. A judge can observe that a human constraint or typed action changes the structured result, while the authoritative state remains protected. This is the foundation for the next persistence/API slice: create, simulate and compare scenarios through tenant-scoped application ports.

## Verification

`go test ./internal/domain/scenario -v -count=1 -timeout=20s` passes deterministic, immutability, risk and stale-context tests. Full backend and frontend validation remains required after the persistence/API slice is added.
