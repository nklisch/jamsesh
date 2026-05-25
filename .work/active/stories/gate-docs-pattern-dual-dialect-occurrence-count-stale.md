---
id: gate-docs-pattern-dual-dialect-occurrence-count-stale
kind: story
stage: drafting
tags: [documentation]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: docs
created: 2026-05-24
updated: 2026-05-24
---

# dual-dialect-mirror-queries: "32 occurrences" stale (actual 40)

## Drift category
pattern-skill-staleness

## Location
- Doc: `.claude/skills/patterns/dual-dialect-mirror-queries.md:62-64`
  (Example 3 trailing prose)
- Code: `db/queries/sqlite/*.sql` and `db/queries/postgres/*.sql`

## Current doc text
> 32 occurrences of `org_id`/`session_id` filters in each dialect's queries —
> counts match exactly.

## Reality
The actual count is **40** occurrences in each dialect (verified by
`grep -c 'org_id\s*=\|session_id\s*=' db/queries/sqlite/*.sql` and same
for postgres — both sum to 40, so the load-bearing "counts match exactly"
half of the claim is still correct).

The number went up across v0.4.0 because the playground epic added new
WHERE-clauses to sessions, bearers, members, presence, and tombstone
queries.

## Required edit
In `.claude/skills/patterns/dual-dialect-mirror-queries.md:62-64`, change
"32 occurrences" to "40 occurrences" (or, better, drop the absolute number
and keep just "matches exactly across dialects" — the number drifts every
time a query is added, and the parity invariant is the actual pattern).

This is medium-confidence drift — the "counts match exactly" half of the
sentence is still correct so the pattern still teaches what matters. Fix
it for accuracy but it's not a load-bearing bug.
