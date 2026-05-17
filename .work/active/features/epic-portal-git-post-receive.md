---
id: epic-portal-git-post-receive
kind: feature
stage: drafting
tags: [portal]
parent: epic-portal-git
depends_on: [epic-portal-git-storage]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Portal Git — Post-Receive Event Emission

## Brief

After the smart-HTTP receive-pack handler has accepted a push (pre-receive
validated, `git-receive-pack` updated the ref), this feature emits the
`commit.arrived` events into the portal event log that the WebSocket gateway
(in `epic-portal-api`) and the auto-merger (in `epic-auto-merger`) consume.

**Inputs** (handed to this feature from the smart-http handler after
receive-pack returns success):

- `session_id`
- For each accepted ref update: `ref_name`, `old_sha`, `new_sha`
- The bare repo handle (from storage)

**Work performed**:

- Resolve the commit range for each ref update (commits in `old_sha..new_sha`)
  using `go-git`
- For each commit: collect `sha`, `author_id` (from trailers), `summary`
  (first line of commit message), `ref`
- Emit a `commit.arrived` event per commit into the event log, with the
  next monotonic per-session `seq` number
- Emit at most one batched event per ref update for ref-meta changes
  (the design pass decides batching strategy)

**Cross-epic touchpoint**: the event log table (`events`) is owned by
`epic-portal-api`. This feature writes to it via the data-layer sqlc query
package. The design pass on this feature is sequenced after portal-api's
events-table feature exists in the substrate (declared as a feature dep then;
intra-epic deps captured here for autopilot readiness).

**Failure handling**: post-receive runs after the ref update has already
committed. Failure to emit an event is logged loudly but does not roll back
the ref update — the source of truth is git. A reconciliation sweep (deferred
out of v1, or shipped as part of the data-layer feature's design) can replay
missed events by diffing the event log against actual git refs.

Does NOT cover the WebSocket fan-out (`epic-portal-api`'s WebSocket gateway
feature subscribes to the event log). Does NOT cover auto-merger triggering
(`epic-auto-merger` subscribes independently).

## Epic context

- Parent epic: `epic-portal-git`
- Position in epic: parallel with pre-receive after storage lands;
  smart-http is the assembly point that calls both.

## Foundation references

- `docs/ARCHITECTURE.md` — Data flow: a turn > post-receive processes the
  push; auto-merger subsection
- `docs/PROTOCOL.md` — WebSocket event types (`commit.arrived` payload
  shape)
- `docs/SPEC.md` — Multi-tenant by design (events carry `org_id` via the
  session lookup)

## Decomposition risks

- The `events` table lives in `epic-portal-api`; this feature writes into
  it without owning the schema. If portal-api's design pass changes the
  event schema, this feature's implementation may need to adapt. Mitigation:
  the design pass on this feature waits until portal-api's events-table
  feature is decomposed (or, if running ahead, locks the canonical
  envelope from `docs/PROTOCOL.md > WebSocket event types` which already
  pins the shape).
- Post-receive after-the-fact failure handling is intentionally weak. A
  missed event means a peer's UI lags one turn — acceptable; git remains
  the source of truth.

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->
