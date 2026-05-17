---
id: e2e-tests-go-module-version-skew
kind: story
stage: drafting
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

## Acceptance criteria

- [ ] Decision documented in `tests/e2e/README.md` (or root `README.md` if it
      ends up at the project level)
- [ ] CI Go version is explicit, not `'stable'`
- [ ] A `go.work` workspace file is considered (and either added with
      rationale or explicitly omitted with rationale)
