---
id: review-store-partition-implementation-note-file-count
kind: story
stage: drafting
tags: [refactor, portal, documentation]
parent: null
depends_on: []
release_binding: null
gate_origin: review
created: 2026-05-24
updated: 2026-05-24
---

# Store-partition implementation notes undercount touched files

## Brief

The implementation notes in
`story-store-partition-handler-signature-sweep.md` and
`feature-refactor-store-narrow-handler-signatures.md` (now both archived)
report "20 files modified across 15 packages" and "19 lowercase
composed interfaces declared".

The actual numbers from `git diff --name-only 2692fd7^..ce5cc9f` are:
- **28 production files** in **14 packages** modified during Step 1
- **20 lowercase composed interfaces** declared (verified with
  `grep -rn "type [a-z][a-zA-Z]*[Ss]tore interface" internal/portal
  --include='*.go' | grep -v _test.go | wc -l`)

This is a bookkeeping slip — the refactor itself is correct and complete.
The archived notes were approved as-is because the divergence is cosmetic
and the substantive claims (zero `store.Store` consumers, build/tests
clean, no behavior change) all hold.

## Why this matters

If anyone consults the archived items later as historical context, the
file-counts misrepresent the blast radius. Git history is the source of
truth; the per-story notes should match.

## Approach

Either (a) accept that the archived items can be wrong and rely on git
history when precision matters, or (b) update the archived bodies with a
correction footnote. Option (a) is more honest — items at `stage: done`
are snapshots of the autopilot's understanding at the time, not
maintained docs.

This is a backlog nit; no urgency.

## Acceptance criteria

- [ ] Decision recorded on whether `stage: done` items get post-hoc
      corrections, or whether git history is the answer.
- [ ] If chosen, the two archived items updated to reflect actual counts.
