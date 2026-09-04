# Challenge-Integrated Architecture Contract

**Status:** Governing delivery baseline
**Verified:** 29 August 2026

This document makes the official WebMCP Challenge requirements part of the project architecture and acceptance process. It does not replace the Official Rules; the official rules and challenge website remain the source of truth if a conflict appears.

## Authority order

1. Official WebMCP Challenge rules and live challenge website;
2. this challenge compliance contract;
3. approved product and architecture documents;
4. implementation details.

## Four equal product score dimensions

| Dimension | Required proof in our product |
|---|---|
| WebMCP Leverage | Browser-native semantic tools are discovered and invoked for real business capabilities, with structured results and meaningful state interaction |
| Execution | An unfamiliar judge can complete the entire flow from business context to approved/committed decision |
| Potential Impact | A founder/operator solves a specific growth-planning problem with less fragmented work and visible trade-offs |
| Creativity & Ambition | Human judgment materially changes agent exploration inside a shared business workspace |

## Architecture consequences

- WebMCP is a first-class browser capability, not a backend transport or marketing label.
- Every tool must map to a meaningful business capability and produce a judge-visible result.
- The backend must preserve the human approval boundary even when a browser agent invokes a tool.
- The MVP optimizes one polished end-to-end workflow over breadth.
- Seed data, deterministic calculations, failure recovery and clear testing instructions are product infrastructure, not optional demo polish.
- English-facing submission and testing materials are required even if development communication is Uzbek.
- Public deployment and public repository/license readiness are release gates.

## Release gates

### Viability gate

- live app fits the WebMCP theme;
- live app exposes working WebMCP tools in the target runtime;
- no critical path depends on unsupported Resources/Prompts or backend MCP transports.

### Product gate

- Orient → Define → Investigate → Explore → Collaborate → Approve → Commit is completable;
- human constraint changes structured output, not only chat text;
- current/proposed/approved/committed states are visually and technically distinct;
- final result is a useful structured business artifact.

### Trust gate

- no AI output directly mutates canonical state;
- approval is version-bound;
- stale, unauthorized, duplicate and failed commits are rejected safely;
- secrets and sensitive payloads do not enter client bundles or logs.

### Submission gate

- public live URL works in ChatGPT’s in-app browser and Chrome WebMCP runtime;
- public repository includes an open-source license;
- English testing instructions are complete;
- demo video is under three minutes, includes audio, and visibly explains WebMCP;
- submission snapshot is frozen at the official deadline.

## Decision rule

Any feature, infrastructure component or abstraction that does not improve at least one score dimension or a release gate is rejected from the MVP unless it is required for security or correctness.

## Quality allocation

WebMCP implementation and judge-visible human-agent collaboration receive exceptional, N1-level design and engineering attention. Other areas remain intentionally narrow, but security, correctness, state integrity and failure safety are never traded away for speed.

Detailed priority tiers: [PRIORITY_MODEL.md](PRIORITY_MODEL.md).
