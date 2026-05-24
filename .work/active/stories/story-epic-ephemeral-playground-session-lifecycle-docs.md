---
id: story-epic-ephemeral-playground-session-lifecycle-docs
kind: story
stage: implementing
tags: [documentation, playground]
parent: feature-epic-ephemeral-playground-session-lifecycle
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# SPEC.md + SECURITY.md roll-forward for playground lifecycle

## Scope

Story 5 of the parent feature. Fills in the placeholders in SPEC.md's
ephemeral-playground section (idle timeout, hard cap, abuse caps were
all left as "TBD" or env-var-only references at scope-time and need
concrete defaults now) and adds the abuse-vector threat model to
SECURITY.md.

This is a docs-only story — no Go code. Independent of all other
session-lifecycle stories.

## Files delivered

- `docs/SPEC.md` — Ephemeral playground sessions subsection updated with:
  - Concrete idle timeout default (30 min)
  - Concrete hard cap default (24 h)
  - Concrete abuse caps: per-IP create rate (3/hour), max participants (5),
    max content size (50 MiB)
  - Destruction trigger semantics (both timers visible to participants)
- `docs/SECURITY.md` — new section "Abuse model for playground sessions"
  covering:
  - Per-IP rate limit rationale (3/hour balances spam-prevention vs
    legitimate evaluation)
  - Content-size cap as both abuse-prevention and storage-cost guard
  - Joiner overflow cap as DoS-prevention
  - Cross-reference to anon-bearer feature's bearer-leak section
    (no duplication; that doc is owned by anon-bearer's docs unit)

## Acceptance criteria

- [ ] SPEC.md ephemeral-playground subsection has every value pinned
      with the concrete defaults from this design (no remaining "TBD" or
      "exact policy decided later" prose)
- [ ] SECURITY.md has the "Abuse model for playground sessions" section
      with the four sub-headings above
- [ ] Both docs read cleanly within their surrounding prose; present
      tense; no "previously" / "in v0.4.0" framing (rolling-foundation
      principle)
- [ ] Cross-reference to anon-bearer's bearer-leak section uses an
      anchor link or section name, not a hard URL

## Notes for the implementing agent

- This story is fully independent — no Go code, no cross-cutting deps.
  Can be implemented before, during, or after any other session-lifecycle
  story.
- Reference the parent feature body's "Story 5" section for the exact
  defaults and the rationale for each.
- If the abuse-cap defaults shift during implementation (e.g. Story 3's
  implementer chooses different values after looking at real-world
  patterns), update SPEC.md and SECURITY.md to match. The docs are the
  source of truth for the rolling-foundation; runtime values must match.
