---
id: gate-docs-pattern-per-package-clock-package-count-undercount
kind: story
stage: implementing
tags: [documentation]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: docs
created: 2026-05-24
updated: 2026-05-24
---

# Pattern skill `per-package-clock-interface.md` undercounts the package set

## Drift category
pattern-skill-staleness

## Location
- Doc: `.claude/skills/patterns/per-package-clock-interface.md:60-61`
- Code: 14 packages declare `type Clock interface { Now() time.Time }` (see body for list)

## Current doc text
> Replicated identically in `internal/portal/{auth, tokens, events, automerger, accounts, mcpendpoint, finalize}` — 10 packages total.

## Reality
Bundle's `feature-refactor-per-package-clock-compliance` brought four more packages onto the per-package-clock pattern: `playground`, `ratelimit`, `storage/objectstore`. Combined with `auth/magic_link.go` and `wsgateway/clock.go` (which already had clocks), the actual count is 14: `accounts/handlers.go, auth/magic_link.go, automerger/outcomes.go, comments/service.go, events/log.go, finalize/handler.go, mcpendpoint/handler.go, playground/clock.go, ratelimit/clock.go, sessions/clock.go, storage/objectstore/clock.go, storage/service.go, tokens/service_impl.go, wsgateway/clock.go`. The example file ref `internal/portal/comments/service.go:27` is also stale — the `Clock interface` declaration moved to line 33 after `story-comments-service-use-slog-not-stdlib-log` reshuffled the file.

## Required edit
Update the package list to the 14 above and the count to "14 packages total" in line 61. Update the `comments/service.go:27` reference at line 24 to `comments/service.go:33`.
