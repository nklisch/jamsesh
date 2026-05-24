---
id: e2e-audit-playground-rest-zero-coverage
kind: story
stage: drafting
tags: [testing, e2e-test, audit, playground]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# Playground REST surface has zero references in `tests/e2e/` — entire v0.4.0 headline feature unverified end-to-end

## Severity
Critical

## Finding type
missing-taxonomy-layer

## Evidence

```
$ grep -rIn -E "playground|anonymous|anon\b|/api/playground|jamsesh jam" tests/e2e/
(no output — zero hits)
```

The 17 golden, 14 failure, 8 chaos, and 4 fuzz tests inventoried in
`tests/e2e/` cover authenticated-org sessions, OAuth onboarding,
lease/fencing, hydration/handoff, object storage, router, MCP routing, and
metrics — but contain **no reference whatsoever** to the playground
subsystem shipped in v0.4.0:

- `POST /api/playground/sessions`
- `POST /api/playground/sessions/{id}/join`
- `GET  /api/playground/sessions/{id}`
- `GET  /api/playground/sessions/{id}/tombstone`
- The `jamsesh_anon_*` bearer scheme
- The reserved `playground` org
- The destruction worker
- Per-IP create rate limiting
- The 50 MiB per-session content cap

All test coverage today lives at unit scope in
`internal/portal/playground/*_test.go` (handler_test.go ≈ 1300 lines,
destruction_test.go, worker_test.go, ratelimit_test.go, provision_test.go).
Those tests use `httptest.NewServer` + a `stubStorage` map + a `fixedClock`
— they exercise no real git, no real Postgres, no real wall clock, no real
binary, no real network.

## Why this matters

A whole shipping feature with **zero end-to-end coverage** means every
production playground regression — bearer issuance against a real DB,
content-cap enforcement at a real pre-receive hook, destruction worker
against a real `ticker-sweep-loop` (per `.claude/rules/patterns.md`),
tombstone serving after real Postgres tx commit — will only be caught by
users hitting prod. The unit tests cannot catch wiring bugs, mistake
between `jamsesh_anon_` and `Bearer` prefix in real chi middleware, mistake
between `playgroundOrgID` constant and the actual provisioned org row, or
mistake between fixed-clock idle-timeout math and `time.Now()` skew. The
`per-package-clock-interface` pattern means a single forgotten clock
injection silently splits behavior between unit and prod.

The remaining nine findings in this audit decompose this gap into
remediable stories. This finding is the umbrella.

## Suggested remedy

This finding does not propose a remedy of its own — it inventories the
problem. The remedy is the bundle of nine follow-on stories tagged
`[e2e-audit]`. Treat this story as the **rollup** parent for them when the
implementation epic is scoped (e.g. `epic-e2e-playground-coverage`). On
that scope, this story closes when child stories' coverage matrix shows at
least one golden test per dimension (playground REST, CLI jam,
destruction, bearer auth).
