---
id: review-per-package-clock-rolling-foundation-prose
kind: story
stage: implementing
tags: [documentation, cleanup]
parent: null
depends_on: []
release_binding: null
created: 2026-05-24
updated: 2026-05-24
---

# Trim "v0.4.0 feature-refactor-per-package-clock-compliance" prose from per-package-clock pattern skill

## Origin
Spawned during review of `gate-docs-pattern-per-package-clock-package-count-undercount`.

## Issue
`.claude/skills/patterns/per-package-clock-interface.md:60-66` reads:

> The `auth/magic_link.go` and `wsgateway/clock.go` packages had this shape
> from inception; the `playground`, `ratelimit`, and `storage/objectstore`
> packages were brought onto the pattern by
> `feature-refactor-per-package-clock-compliance` in v0.4.0.

That second clause names a past version and a substrate item — a
rolling-foundation violation. The current package list is enough; readers
do not need to know which version added each entry.

## Fix
Drop the second sentence ("the `playground`, `ratelimit`, …"). Leave the
14-package list and the count; the doc should describe the system now,
not the journey to it.
