---
id: portal-dep-failure-error-codes-protocol-doc
kind: story
stage: review
tags: [portal, documentation]
parent: portal-dep-failure-error-codes
depends_on: [portal-dep-failure-error-codes-envelope-helper]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Register `dep.*` codes in `docs/PROTOCOL.md > HTTP error contract`

Foundation-doc roll-forward for the new dep-class error taxonomy.

**This story's edit is rolled forward in the design-commit** per the
rolling-foundation principle: the doc must describe the contract NOW,
which means the codes are registered the moment the feature's design
is committed. The story exists as a paper-trail breadcrumb so the doc
change is reviewable as a substrate item alongside the wiring stories,
and so subsequent doc-drift gate scans can attribute the edit to a
named item.

## What rolls forward

`docs/PROTOCOL.md > HTTP error contract` gains a new sub-section that
enumerates the `dep.*` codes, their HTTP status, the
`Retry-After` semantics, and a one-line description for each. The
existing list of `Common error codes` (auth.*, session.*, push.*,
fork.*, oauth.*) is extended with the four dep codes.

The wording is descriptive of the NOW state — no "previously this
was plain text" notes anywhere.

## Files

- `docs/PROTOCOL.md` — the edit was performed in the design commit
  (commit `design: portal-dep-failure-error-codes`). No further edits
  expected from this story at implementing time.

## Verification

- [ ] `docs/PROTOCOL.md` lists all four `dep.*` codes under
      `HTTP error contract`
- [ ] Each entry specifies HTTP status (503 for the upstream-down
      trio, 500 for git subprocess)
- [ ] Each 503 entry specifies the `Retry-After` value
- [ ] No "previously" / "v1.x" / "before this change" prose anywhere

## Risk

NONE — doc-only.

## Rollback

`git revert` the design commit's `docs/PROTOCOL.md` portion.

## Implementation notes

Verified that `docs/PROTOCOL.md > HTTP error contract > Dependency-failure
codes` (added in the design commit `ecc9f78 design:
portal-dep-failure-error-codes`) matches the feature design's promised
taxonomy exactly:

- `dep.smtp_unavailable` — 503, `Retry-After: 5`
- `dep.db_unavailable` — 503, `Retry-After: 2` (with the
  `store.ErrNotFound` / `store.ErrUniqueViolation` carve-out noted)
- `dep.oauth_provider_unavailable` — 503, `Retry-After: 10`
- `dep.git_subprocess_failed` — 500, no `Retry-After`

The doc also captures the rationale (503 communicates retryability;
`error` code disambiguates which dep is down) and the operator-logging
contract (underlying cause logged at error level; never leaked into the
response body). No "previously this was…" prose is present anywhere in
the section.

No further edits to `docs/PROTOCOL.md` are required from this story. All
verification checklist items above are satisfied by the design-commit
state.
