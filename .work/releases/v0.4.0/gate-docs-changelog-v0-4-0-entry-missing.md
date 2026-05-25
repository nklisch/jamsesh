---
id: gate-docs-changelog-v0-4-0-entry-missing
kind: story
stage: done
tags: [documentation]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: docs
created: 2026-05-24
updated: 2026-05-24
---

# CHANGELOG.md has no v0.4.0 entry yet

## Drift category
changelog-gap

## Location
- Doc: `CHANGELOG.md:1` (latest entry is v0.3.1)
- Code: 80 bound items in `/tmp/bound-ids-v0.4.0.txt`

## Current doc text
> Top entry is `## v0.3.1` (2026-05-21); no `## v0.4.0` heading present.

## Reality
80 substrate items bound to v0.4.0 are shipping (epic-ephemeral-playground end-to-end, multiple refactor features, spec-discipline feature, foundation-doc rollups). The Changelog still ends at v0.3.1.

## Required edit
Composed in release-deploy Phase 5.5 — group by category (Features / Fixes / Refactor / Performance / Security / Documentation / Internal), one bullet per logical change. This item is resolved as part of the release-deploy changelog composition step.

## Implementation notes

Composed inline as part of the v0.4.0 implement-orchestrator pass (since
the orchestrator was draining this release-binding's `implementing` band
and this story was the last item in the queue).

The v0.4.0 entry follows the same category structure as the v0.3.x
entries: a 1-2 paragraph headline summary, then Features / Fixes /
Security / Refactor / Documentation / Internal. The 80 bound items
collapse to ~30 bullets — each feature gets one bullet, each refactor
gets one bullet, and the long tail of gate-produced findings is rolled
into a couple of summary lines under Internal rather than enumerated
one-by-one.

Parked items (`bug-playground-join-with-nickname-returns-410-on-fresh-session`,
the 40 medium-severity drafting items) are noted in the Internal
section so the audit trail is honest about what didn't ship.

`CHANGELOG.md:3` now has `## v0.4.0` heading; the previous top entry
(`## v0.3.1`) is preserved verbatim below.
