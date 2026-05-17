---
id: epic-auto-merger
kind: epic
stage: drafting
tags: [portal]
parent: null
depends_on: [epic-portal-git]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Auto-merger

## Brief

The continuous-integration heart of jamsesh. A server-side worker (Go
goroutines triggered by post-receive events) that processes every commit
arriving on a sync-mode ref:

1. Resolves the commit's parent and the current `draft` tip.
2. Finds the common ancestor.
3. Runs a three-way merge in-process using `go-git`.
4. On success: creates a merge commit (with the new commit and the draft
   tip as parents) carrying `Auto-Merger: true` and `Source-Commit: <sha>`
   trailers, advances `draft`, emits `merge.succeeded`.
5. On conflict: does not advance `draft`, emits `conflict.detected` with
   structured payload (file paths, line ranges, all three SHAs).

Parses `Resolves-Conflict: <event-id>` trailers on incoming commits — when
such a commit succeeds in the auto-merger, the named conflict event is
marked resolved automatically.

Respects mode: isolated refs are skipped entirely. The auto-merger only
operates on sync refs.

This epic does NOT cover the in-portal-UI conflict-resolver (there isn't
one — humans resolve conflicts in their own CC environment); it does NOT
cover finalize-time cherry-picks (those happen locally on the human's
machine).

## Foundation references

- `docs/ARCHITECTURE.md` — The auto-merger section
- `docs/SPEC.md` — Ref structure (`draft` ref semantics)
- `docs/PROTOCOL.md` — Conflict event schema, Commit trailer conventions
- `docs/PRINCIPLES.md` — Liveness via continuous integration

## Design decisions

- **Conflict-resolution heuristics**: auto-resolve safe cases; escalate
  everything else. "Safe" is precisely defined to keep the auto-merger
  honest. Auto-resolve only:
  - **Whitespace-only conflicts** — trailing whitespace differences,
    line-ending differences (LF/CRLF), tab-vs-space changes that don't
    affect indentation depth
  - **Non-overlapping additions** within the same conflict hunk where
    both sides ADD different lines and neither modifies or deletes a
    shared line (interleave both sides in the order they appear)
  - **Identical edits** — both sides made the same change (textually
    equal post-merge)
  Escalate to `conflict.detected` for any conflict involving:
  - Both sides modifying the same line(s) differently
  - One side deleting + other side modifying
  - Rename + modification interactions (let git's rename detection
    surface these as conflicts)
  - Any case where the resolution would be a judgment call

  Auto-resolved merge commits carry an extra trailer:
  `Auto-Resolved: whitespace` / `additions` / `identical` so the
  resolution heuristic is auditable from `git log`.

- **Merge-commit author identity**: author = the source commit's author
  (the human whose work is being integrated); committer = synthetic
  `jamsesh auto-merger <auto-merger@<portal-host>>` identity; trailer
  `Auto-Merger: true` + `Source-Commit: <sha>`. This uses git's
  canonical author/committer distinction correctly: Alice wrote the
  change, auto-merger applied it to draft. `git log` reads naturally
  ("alice: Add refresh-token revocation endpoint") while still being
  machine-distinguishable as auto-merger-generated via the committer
  field and the trailers. The auto-merger is invisible plumbing — its
  identity surfaces only in the committer field and trailers, never as
  the author of the integration work.

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->


## Anticipated child features

Provisional — actual decomposition lands when this epic is designed.

- Auto-merger worker runtime (post-receive event subscription, per-session
  serialization to avoid concurrent draft mutations)
- Three-way merge via go-git (theirs/ours/base resolution)
- Merge-commit creation with `Auto-Merger` + `Source-Commit` trailers
- Conflict event emission with structured payload (paths, ranges, SHAs)
- Mode-aware filtering (skip isolated refs)
- `Resolves-Conflict` trailer parsing and conflict event auto-closure

<!-- Design pass on each child feature will fill in specifics. -->
