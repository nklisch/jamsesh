---
id: story-epic-ephemeral-playground-session-lifecycle-docs
kind: story
stage: done
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

## Implementation notes

`docs/SPEC.md` — replaced the placeholder "End:" bullet in the Ephemeral
playground sessions section with concrete destruction-trigger semantics
(whichever of idle/hard-cap fires first, both timers visible as a countdown
badge). Added a new `#### Playground session limits and defaults` subsection
with a table of all five limits and their env vars, followed by a prose
paragraph for each (idle timeout, hard cap, participant cap, per-IP rate limit,
content-size cap). No remaining "TBD" or "exact policy decided later" text.

`docs/SECURITY.md` — added a new `## Abuse model for playground sessions`
section (four sub-headings) immediately before `## Audit trail`:
1. Per-IP rate limit rationale — 3/hour balance, token-bucket via
   `internal/portal/ratelimit`, join requests deliberately excluded.
2. Content-size cap — dual purpose (abuse prevention + storage-cost guard),
   enforced at pre-receive by `CheckPlaygroundCaps`.
3. Joiner overflow as DoS prevention — 5-cap rationale, hard error semantics,
   TOCTOU note.
4. Cross-reference to the existing "Anonymous session-scoped bearers" section
   by anchor link (no duplication of that section's content).

Story 3 (abuse-caps) was checked: still at `stage: implementing`, no value
changes were applied. All defaults match the feature design exactly.

## Review (2026-05-23)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- `docs/SPEC.md` "Content-size cap" prose says the check "compares the incoming
  packfile's object count against the current session storage total reported by
  the storage backend" — "object count" vs "storage total (bytes)" mixes units.
  `docs/SECURITY.md`'s "Content-size cap" section phrases it cleanly. Editorial
  polish only; not blocking.

**Notes**: Docs-only story. All four acceptance criteria satisfied: SPEC
limits-and-defaults table + per-limit prose has every value pinned with no
remaining "TBD" wording; SECURITY's "Abuse model for playground sessions"
section has the four required sub-headings; prose is present-tense with no
version-stamped framing; cross-reference uses an anchor link
(`#anonymous-session-scoped-bearers`, which resolves to an existing heading at
SECURITY.md:258). All env var names and error codes
(`playground.session_full`, `playground.size_exceeded`) are consistent with the
parent feature design and sibling stories. Tombstone semantics (30-day TTL,
summary page) align with SPEC.md:217 and the REST-endpoints story's tombstones
table TTL. Advancing review → done.
