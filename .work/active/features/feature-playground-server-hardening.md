---
id: feature-playground-server-hardening
kind: feature
stage: drafting
tags: [portal, playground]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# Playground server hardening

## Brief

Three review-surfaced server-side issues in `internal/portal/playground/`
bundled into one feature for a shared design pass. All three were
filed during review of
`story-epic-ephemeral-playground-session-lifecycle-rest-endpoints`
(rest endpoints) and the parent feature `feature-epic-ephemeral-playground-session-lifecycle`.

## Why a feature

The three children share a code area and one of them carries a
cross-cutting refactor decision worth a single design pass:
extract `validateWritableScope` from `internal/portal/sessions/`
into a shared package (proposed: `internal/portal/sessionscope/`)
importable from both the durable session handler and the playground
handler. Bundling under one feature gives the work a coherent verdict
and a clean PR shape.

## Child stories

- `story-playground-server-hardening-wordlist-dedup` — dedupe 62
  duplicate adjectives in
  `internal/portal/playground/wordlist/adjectives.txt`
- `story-playground-server-hardening-handler-test-coverage` — add 3
  missing handler tests + refactor `openStore(t)` into a `stores(t)`
  helper so every test exercises both SQLite and Postgres
- `story-playground-server-hardening-writable-scope-validation` —
  validate the `Scope` field on `CreatePlaygroundSession` and extract
  `validateWritableScope` into a shared package

## Design notes (for /agile-workflow:feature-design)

The interesting design decision is the home of the extracted
`validateWritableScope` helper. Candidates:

1. New `internal/portal/sessionscope/` package — clean separation,
   imported by both `sessions` and `playground`
2. Move into `internal/portal/storage/` alongside other shared
   primitives
3. Promote to a higher-level helper inside an existing shared package

Option 1 looks correct on the surface (single-responsibility, named
for what it protects). `feature-design` should confirm by checking
what else might want to live in such a package (e.g., scope parsing,
scope normalization).

The handler-test-coverage story also lands a cross-cutting test
refactor (`openStore(t)` → `stores(t)`) — feature-design should
sequence it after the validation story so the new tests for
validation use the dialect-aware harness from the start.

## Design decisions

- **Home of extracted `validateWritableScope`**: extend `internal/portal/prereceive/` — that package already owns `CompileScope` (exported) and a private `parseWritableScope` (parse-only, no glob). Consolidating all scope-related primitives there avoids a new single-function package, accepts a mild responsibility blur (prereceive becomes both push-time validator and write-time validation provider), and gives sessions + playground a single import edge.
- **`stores(t)` test helper**: consolidate into a shared test-helper package — canonical `stores(t)` (currently in `internal/db/store/helpers_test.go` with truncateAll cleanup) moves to a shared location (e.g. `internal/db/store/testharness/` or a test-tagged file in the store package), so the existing duplicate in `internal/portal/playground/provision_test.go` and the new 3rd usage in `playground/handler_test.go` import one source. The drift risk (one version has cleanup, the other doesn't) gets fixed in the same pass.
- **Implementation order**: stores(t) refactor first → writable-scope-validation → wordlist-dedup (parallel-safe). The handler-test-coverage story delivers the shared `stores(t)` helper as part of its work; the validation story's new tests then use the dialect-aware harness from the start and get Postgres coverage for free. This reverses the literal reading of the original feature body — feature-design confirms the intent was "harness first" so downstream tests inherit it.

## Acceptance (rollup)

- All three child stories at stage:done with verdicts ≥ approve-with-comments
- `validateWritableScope` lives in `internal/portal/prereceive/` and is imported
  by both `internal/portal/sessions/` and `internal/portal/playground/`
- `stores(t)` is consolidated into one location and consumed by sessions tests,
  playground provision tests, and playground handler tests
- `go test ./internal/portal/playground/...` passes against both
  SQLite and Postgres
- `sort internal/portal/playground/wordlist/adjectives.txt | uniq -c | awk '$1>1'`
  returns no rows
