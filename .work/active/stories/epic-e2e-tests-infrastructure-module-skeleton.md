---
id: epic-e2e-tests-infrastructure-module-skeleton
kind: story
stage: implementing
tags: [e2e-test, testing]
parent: epic-e2e-tests-infrastructure
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# E2E Infra — Module skeleton + Makefile entry

## Scope

Stand up the `tests/e2e/` Go module and the Makefile entrypoint so
subsequent stories have a place to land.

## Files to create / modify

- `tests/e2e/go.mod` — new module `e2e` requiring at minimum
  `github.com/stretchr/testify` (already used in the project) and a
  Go directive matching the root `go.mod` (`go 1.25.7`)
- `tests/e2e/README.md` — short doc: how to run, where containers
  come from, link back to `.work/active/features/epic-e2e-tests-infrastructure.md`
- `tests/e2e/scaffolding/placeholder_test.go` — single test
  `TestE2EModuleBuilds(t *testing.T) { /* intentionally empty */ }`
  proving the module compiles and runs through `go test`
- `Makefile` — append new `.PHONY` line and add `test-e2e`,
  `test-e2e-go`, `test-e2e-playwright` targets per the design unit 1

## Acceptance criteria

- [ ] `cd tests/e2e && go test ./...` runs green (the placeholder
      test passes trivially)
- [ ] `make test-e2e-go` from repo root runs the same suite
- [ ] `make test-e2e` runs `test-e2e-go` then `test-e2e-playwright`
      — the playwright half is allowed to no-op cleanly (target is
      idempotent against a missing `tests/e2e/playwright` directory)
- [ ] `tests/e2e/README.md` documents the suite's purpose, how to
      run locally, and the link back to the feature item
- [ ] The new module does NOT add any deps to the root `go.mod`

## Notes for the implementer

- Keep the `go.mod` deps minimal — Testcontainers-Go and friends
  land in the testcontainers-fixtures story; this story is bones
  only
- The placeholder test must actually run (not skip) so a future
  developer running `go test ./...` sees a passing test, not a
  silent zero-test no-op
- The playwright Makefile target should gracefully handle the
  absence of `tests/e2e/playwright/` (e.g. `test -d tests/e2e/playwright
  && cd tests/e2e/playwright && ... || echo "playwright not bootstrapped
  yet, skipping"`)
