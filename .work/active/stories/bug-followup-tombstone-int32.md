---
id: bug-followup-tombstone-int32
kind: story
stage: implementing
tags: [bug, portal, data-layer]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
bug_origin: gate
bug_severity: low
bug_domain: data-layer
bug_location: db/schema/postgres.sql:284
---

# Tombstone aggregate fields are 32-bit in Postgres (same class as the seq bug)

Surfaced by the codex final-gate review of `epic-bug-squash` (out of scope for
that epic — not one of the 28 scanned findings). The tombstone aggregate
fields (~`db/schema/postgres.sql:284`) are Postgres `INTEGER` with `int32` casts
in the adapter, while the domain model treats them as `int64` — the same
schema/domain-type mismatch class that `bug-squash-postgres-seq-32bit` fixed for
`events.seq`. Widen to `BIGINT` (non-destructive) + drop the `int32` casts +
regen sqlc + forward goose migration, mirroring the seq fix. Low severity
(unrealistic to overflow at current scale) but breaks the isomorphic-surface
contract.
