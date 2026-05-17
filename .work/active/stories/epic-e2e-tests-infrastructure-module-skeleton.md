---
id: epic-e2e-tests-infrastructure-module-skeleton
kind: story
stage: done
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

## Implementation notes

Files created:
- `tests/e2e/go.mod` — module `jamsesh/tests/e2e`, `go 1.25.7`, no deps (intentionally minimal; Testcontainers-Go lands in the testcontainers-fixtures story)
- `tests/e2e/scaffolding/placeholder_test.go` — package `scaffolding_test`, `TestE2EModuleBuilds` passes trivially; uses stdlib `testing` only, no testify
- `tests/e2e/README.md` — documents how to run, container provenance, links back to the feature item
- `Makefile` — appended `test-e2e`, `test-e2e-go`, `test-e2e-playwright` targets with a separate `.PHONY` line

Deviations from story body:
- The story body mentioned adding `github.com/stretchr/testify` as a dep; the implementation brief and notes override this — `go.mod` is intentionally dep-free at this stage. The placeholder test uses stdlib `testing` only.

Verification:
- `cd tests/e2e && go test ./...` → `ok jamsesh/tests/e2e/scaffolding`
- `make test-e2e-go` → passes
- `make test-e2e` → Go tests pass, playwright no-ops with "playwright not bootstrapped yet, skipping"
- `git diff go.mod` → empty (root module untouched)

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- The original Makefile target used `@test -d X && (cd X && npm test) || echo skip` — in shell, `(A && B) || C` would have silenced npm test failures and triggered the skip-echo. Wave 4 (`playwright-bootstrap`) replaced it with proper `if [ -d ]; then ...; else ...; fi` semantics, so this is corrected in the live tree.
- The original `tests/e2e/README.md` referenced "Dex OIDC" — the parent feature's mock plan uses WireMock, not Dex. Wave 3 rewrote the README with the correct fixture table.

**Notes**: Both nits self-corrected via later waves in the same feature. Verified by inspecting the current `Makefile` and `tests/e2e/README.md` — neither retains the issues from the original commit.
