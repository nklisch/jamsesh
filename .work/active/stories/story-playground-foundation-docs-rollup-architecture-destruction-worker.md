---
id: story-playground-foundation-docs-rollup-architecture-destruction-worker
kind: story
stage: implementing
tags: [documentation, playground]
parent: feature-playground-foundation-docs-rollup
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# Add playground destruction worker to ARCHITECTURE.md Components

## Source

Feature-level review of `feature-epic-ephemeral-playground-session-lifecycle`
(2026-05-23). The feature design's "Foundation references" section asserted:

> `docs/ARCHITECTURE.md` § Components — destruction worker is a new
> background-goroutine subsystem inside the portal binary; its responsibility
> line is added to ARCHITECTURE.md by this feature

This roll-forward never happened. Story 5 (docs) updated `SPEC.md` and
`SECURITY.md` per its own scope but did not touch `ARCHITECTURE.md`. The
gap fell between stories — no story owned the Components-list update.

## What's drifted

`docs/ARCHITECTURE.md` § Components (lines ~37–86) lists the portal's
subsystems including:

- REST API
- MCP endpoint
- Git smart-HTTP
- **Auto-merger workers** (background goroutines triggered by post-receive)
- WebSocket gateway
- Data store

A parallel entry for the playground destruction worker is missing. The
worker is a real background-goroutine subsystem (started in
`cmd/portal/main.go` when `cfg.PlaygroundEnabled`), runs on a configurable
interval (`JAMSESH_PLAYGROUND_SWEEP_INTERVAL_S`, default 60s), and executes
the 8-step destruction cascade in `internal/portal/playground/destruction.go`.

SPEC.md correctly describes the destruction semantics; ARCHITECTURE.md
should describe the subsystem topology (a separate goroutine that sweeps
expired playground sessions, parallel to auto-merger workers).

## Fix

Add a `**Playground destruction worker**` bullet to the portal Components
list, paralleling the Auto-merger workers entry. Suggested text:

> **Playground destruction worker** — single background goroutine (started
> when `JAMSESH_PLAYGROUND_ENABLED=true`) that sweeps active playground
> sessions on a configurable interval (`JAMSESH_PLAYGROUND_SWEEP_INTERVAL_S`,
> default 60s). For each session past its idle or hard-cap deadline, runs the
> destruction cascade — record tombstone, revoke bearers, delete session
> rows (FK cascades members + events + presence + bearers), delete anonymous
> accounts, remove the bare repo from disk. Idempotent across steps;
> partial-failure resumption on the next tick. Periodic tombstone-TTL purge
> runs every 60th tick.

Optionally also update the ASCII portal block diagram (lines ~20–31) to
mention the destruction worker alongside the auto-merger workers, though
that's lower priority.

## Acceptance criteria

- [ ] `docs/ARCHITECTURE.md` Components list has a "Playground destruction
      worker" entry
- [ ] Entry describes the goroutine topology, interval config knob,
      destruction cascade summary, and idempotency stance
- [ ] Reads cleanly in present tense (rolling-foundation principle)
- [ ] No "previously" or "newly added" framing

## Notes

- Not a blocker — ARCHITECTURE.md isn't asserting anything false, only
  incomplete. The drift is a gap rather than a contradiction.
- Cross-reference for the writer: the worker lives at
  `internal/portal/playground/worker.go` and the cascade at
  `internal/portal/playground/destruction.go`. Boot wiring is in
  `cmd/portal/main.go` near the auto-merger wiring.
