---
id: bug-squash-postgres-seq-32bit
kind: story
stage: drafting
tags: [bug, portal, data-layer]
parent: epic-bug-squash-data-tx-integrity
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
bug_origin: scan
bug_severity: low
bug_domain: data-layer
bug_location: db/schema/postgres.sql:118
---

# Per-session event seq is 32-bit in Postgres but int64 everywhere else (dialect divergence)

**Location**: `db/schema/postgres.sql:118` (and `:111`; adapter casts `internal/db/store/postgres_adapter.go:761`) · **Severity**: low · **Pattern**: dual-dialect-mirror column-type divergence

`events.seq` and `event_seq.next` are `INTEGER` (signed 32-bit, max 2,147,483,647) in Postgres, while the Go domain model and the SQLite path treat `seq` as `int64`, and the postgres adapter casts `int32(p.Seq)` before insert. A session whose monotonic per-stream counter exceeds 2^31 wraps/truncates silently on Postgres only, corrupting event ordering. Blast radius unrealistic for normal sessions (hence low), but it breaks the isomorphic-surface contract. Fix: make both columns `BIGINT` in `db/schema/postgres.sql`, regenerate sqlc so `AllocateNextSeq*` return `int64`, and drop the `int32` casts in the postgres adapter.

```sql
CREATE TABLE events ( ... seq INTEGER NOT NULL, ... );      -- 32-bit; sqlite + domain use int64
CREATE TABLE event_seq ( ... next INTEGER NOT NULL DEFAULT 0 );
```
