---
id: e2e-tests-go-module-version-skew
kind: story
stage: review
tags: [e2e-test, testing, documentation]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# tests/e2e/go.mod declares go 1.26 while root declares go 1.25.7

## Finding

- Root `/home/nathan/dev/jamsesh/go.mod` declares `go 1.25.7`
- `/home/nathan/dev/jamsesh/tests/e2e/go.mod` declares `go 1.26`

The e2e module was bumped (likely transitively by `testcontainers-go v0.42.0`)
during the testcontainers-fixtures implementation. The CI workflow uses
`actions/setup-go@v5` with `go-version: 'stable'`, which currently resolves
to a version satisfying both modules. But the skew is undocumented as a
project decision.

## Why it matters

- A developer cloning the repo with an older Go (e.g. 1.25.x exactly) can
  `go build` the root but fails when running e2e tests
- A future Go release could deprecate 1.26 features assumed by the e2e
  module
- The skew is invisible from the root `go.mod` — easy to miss in PR review

## Suggested resolution

Pick one and document the choice in `tests/e2e/README.md`:

1. **Pin the e2e module to a specific Go version** matching the root, and
   work around the testcontainers-go requirement (potentially impossible
   if 1.26-specific features are used)
2. **Bump the root `go.mod` to 1.26** for consistency. Verify no production
   build infrastructure breaks
3. **Document the skew as intentional** — note in `tests/e2e/README.md` that
   the e2e suite requires Go 1.26+, and update CI to use an explicit
   `go-version: '1.26'` rather than `'stable'`

## Decision

**Bump the root `go.mod` to 1.26** (option 2). Both modules now declare
`go 1.26`; CI pins `go-version: '1.26.x'` explicitly. No skew to
document anymore.

Why option 2:
- Eliminates the skew at the source rather than annotating around it.
- Root `go.mod` is the reference for everything else (Dockerfile,
  release workflow); having it match the e2e module reduces
  "wait, which version?" mental overhead.
- `go 1.26` was released in early 2026; no production-build infra
  breakage was introduced (verified `go build ./...` from repo root
  passes).

Why not option 1 (pin e2e back): `testcontainers-go v0.42.0` plausibly
relies on 1.26 features; rolling the e2e module back may not even be
possible, and we'd lose access to whatever 1.26 surface the fixture
code wants going forward.

Why not option 3 (document the skew): now that 1.26 is widely
available there's no real reason to keep two versions in flight just
to honor history.

### `go.work` workspace

Considered and **omitted**. Rationale: the two modules don't share any
internal code today — `tests/e2e` is a standalone module that depends
on `jamsesh/...` only via published-internal imports (when applicable)
and via the running portal container. A `go.work` file would primarily
help IDEs that don't auto-discover submodules; current setup with
`gopls`'s default multi-module mode handles both modules fine. Worth
revisiting if the e2e module starts pulling in `internal/portal/...`
directly.

## Implementation notes

### Files touched

- `go.mod` — `go 1.25.7` → `go 1.26`
- `.github/workflows/e2e.yml` — `go-version: 'stable'` → `'1.26.x'`
- `.github/workflows/release.yml` — `GO_VERSION: '1.22.x'` →
  `'1.26.x'` (was 4 versions stale — release builds were running on
  an unsupported Go since the e2e bump)
- `.github/workflows/quickstart.yml` — already uses
  `go-version-file: go.mod`, picks up the bump automatically
- `tests/e2e/README.md` — added a Go 1.26 prerequisite + noted CI
  pin; also dropped the stale "distroless image used in production"
  phrasing (production is now alpine + git per
  `portal-prod-dockerfile-base-image-review`)

### Verification

`go build ./...` from repo root: clean.
`cd tests/e2e && go build ./...`: clean (verified earlier in the
session for `e2e-fixtures-capture-container-logs-on-failure`).

## Acceptance criteria

- [x] Decision documented in `tests/e2e/README.md` — Go 1.26
      prerequisite + CI pin noted in the Prerequisites section
- [x] CI Go version is explicit, not `'stable'` — `e2e.yml` pinned to
      `1.26.x`; `release.yml` GO_VERSION bumped from a 4-version-stale
      `1.22.x` to `1.26.x`
- [x] A `go.work` workspace file is considered — omitted with
      rationale documented above
