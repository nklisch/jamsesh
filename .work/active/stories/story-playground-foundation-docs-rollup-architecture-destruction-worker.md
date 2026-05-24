---
id: story-playground-foundation-docs-rollup-architecture-destruction-worker
kind: story
stage: done
tags: [documentation, playground]
parent: feature-playground-foundation-docs-rollup
depends_on: []
release_binding: v0.4.0
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

Per feature-design (`feature-playground-foundation-docs-rollup`), this
story owns BOTH the Components-list update AND the ASCII diagram update
(the latter was originally marked optional but is now in-scope per the
feature's Design decisions).

### Edit 1 — § System overview ASCII diagram (lines ~7–38)

The portal block currently lists `• Auto-merger workers` on line 28.
Add a parallel bullet immediately beneath:

```
│  • Auto-merger workers   │
│  • Playground destruction│
│    worker                │
```

(Or however best fits within the box width — preserve trailing pipe
alignment. The "Playground destruction worker" label may need to wrap;
either single-line `• Playground destroy worker` or wrapped two-line form
is acceptable as long as ASCII alignment holds. Test by viewing the file
in a monospace editor after the edit.)

### Edit 2 — § Components → Portal (lines ~44–86)

Insert a new bold-prefixed paragraph between the existing
**Auto-merger workers** entry (line 73) and **WebSocket gateway** (line 78):

> **Playground destruction worker** — single background goroutine (started
> when `JAMSESH_PLAYGROUND_ENABLED=true`) that sweeps active playground
> sessions on a configurable interval (`JAMSESH_PLAYGROUND_SWEEP_INTERVAL_S`,
> default 60s). For each session past its idle or hard-cap deadline, runs the
> destruction cascade — record tombstone, revoke bearers, delete session
> rows (FK cascades members + events + presence + bearers), delete anonymous
> accounts, remove the bare repo from disk. Idempotent across steps;
> partial-failure resumption on the next tick. Periodic tombstone-TTL purge
> runs every 60th tick.

## Acceptance criteria

- [ ] `docs/ARCHITECTURE.md` Components → Portal has a "Playground
      destruction worker" entry between Auto-merger workers and WebSocket
      gateway
- [ ] Entry describes the goroutine topology, interval config knob,
      destruction cascade summary, idempotency stance, and tombstone purge
- [ ] System overview ASCII diagram (lines ~7–38) lists the destruction
      worker in the portal block alongside Auto-merger workers; box-art
      alignment preserved
- [ ] Reads cleanly in present tense (rolling-foundation principle)
- [ ] No "previously" or "newly added" framing

## Notes

- Not a blocker — ARCHITECTURE.md isn't asserting anything false, only
  incomplete. The drift is a gap rather than a contradiction.
- Cross-reference for the writer: the worker lives at
  `internal/portal/playground/worker.go` and the cascade at
  `internal/portal/playground/destruction.go`. Boot wiring is in
  `cmd/portal/main.go` near the auto-merger wiring.

## Implementation notes (2026-05-23)

Both edits applied to `docs/ARCHITECTURE.md`:

1. **System overview ASCII diagram (lines 7-38)** — added
   `• Playground destroyer` bullet on line 29 of the portal block. Used the
   short label "destroyer" rather than the full "destruction worker" to keep
   the bullet under the existing 24-char interior width without wrapping —
   matches the same compact register as "Auto-merger workers" / "WS gateway".
   All five portal-block lines maintain the 59-char total line width and
   trailing-pipe alignment (verified by `awk '{print NR" ("length($0)")"}'`).

2. **Components → Portal section** — added the **Playground destruction worker**
   paragraph between **Auto-merger workers** and **WebSocket gateway** (line 79).
   Covers the five required points: goroutine topology, interval config knob
   (`JAMSESH_PLAYGROUND_DESTRUCTION_SWEEP_INTERVAL_S`, default 60s), destruction
   cascade summary, idempotency stance, periodic tombstone-TTL purge cadence.

Verification:
- `grep -n "destruction worker\|Playground destroyer" docs/ARCHITECTURE.md` →
  hits in both the diagram (line 29 as "Playground destroyer") and Components
  (line 79 as "Playground destruction worker").
- `grep -niE "previously|newly added|note: in|used to be" docs/ARCHITECTURE.md` →
  no hits in the edited regions (one pre-existing "previously" still present
  unrelated to this story, not introduced here).
- Present-tense framing throughout — the worker IS, the cascade IS, the
  purge runs every 60th tick.

## Review (2026-05-23)

**Verdict**: Approve with comments (blocker fixed inline)

**Blockers**:
- Foundation-doc drift on env var name — implementation notes and the
  Components paragraph asserted `JAMSESH_PLAYGROUND_SWEEP_INTERVAL_S`, but
  `internal/portal/config/config.go:796` and `config_test.go:452` show the
  real env var is `JAMSESH_PLAYGROUND_DESTRUCTION_SWEEP_INTERVAL_S`. Fixed
  inline in this commit (doc edit + implementation-notes edit). The doc
  now matches the system as it is.

**Important**: none

**Nits**:
- "Playground destroyer" in the ASCII diagram is a slightly more casual
  label than the "Playground destruction worker" used in Components. Not
  changing — short label is necessary for box-width and the longer
  paragraph below provides the formal name.

**Notes**:
- Acceptance criteria all met after env-var correction.
- ASCII alignment verified: all five portal-block lines at 59-char width
  with aligned trailing pipes.
- No "previously"/"newly added" framing introduced.
- Other doc claims verified against source: `JAMSESH_PLAYGROUND_ENABLED`
  matches, 60s default matches `Worker.Interval`, every-60-ticks tombstone
  purge matches `worker.go:67 const purgeEvery = 60`, idempotent +
  next-tick resumption matches `sweep()` continue-on-error semantics, and
  the cascade order matches `destruction.go`.
