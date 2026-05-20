---
id: testing-release-spa-embed-guard
kind: story
stage: implementing
tags: [testing, infra, release, ui]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-19
updated: 2026-05-19
---

# Release pipeline must fail before publishing a portal binary with an empty embedded SPA

## Brief

The reporter discovered that `ghcr.io/nklisch/jamsesh:v0.1.1` ships
with an empty embedded SPA — the release workflow ran `go build`
without first building the frontend, so the `//go:embed all:dist`
directive picked up only the `.gitkeep` placeholder. The published
image is functionally broken: the home page is a stub, the SPA never
loads, and no user can sign in.

A test would have caught this before publication. This story adds a
regression guard that fails the release pipeline if the embedded
`dist/` doesn't contain a real SPA.

The sibling story `bug-release-workflow-missing-frontend-build`
tracks the fix. This story is the guard that ensures the bug stays
fixed. The two should ideally ship in the same release, but neither
blocks the other: the guard alone catches the existing bug (failing
the next release until the workflow is also fixed); the fix alone
restores correctness (but leaves the guard absent, so the next
similar regression won't be caught).

## Strategic decisions

Resolved at scope.

- **Go test, build-tag-gated, not a CI bash check alone.** A Go test
  imports the actual embed package — same FS the running portal will
  serve — so it tests what matters. A bash `strings | grep` check
  alone tests a proxy signal (HTML bytes in the binary), which is
  weaker. The Go test is primary; a `strings` check is a
  belt-and-suspenders second line.
- **Gate with `//go:build release`** (or equivalent tagged file
  separation) so local `go test ./...` on a fresh checkout always
  passes. Fresh checkouts have an empty `dist/` (only `.gitkeep`) and
  shouldn't fail every developer's local test runs. The release
  workflow runs `go test -tags release ./internal/portal/assets/...`
  after `make frontend-build` and before `go build`.
- **Two assertions in the test, not one.** Assert (a) `index.html`
  exists in the embedded FS and (b) at least one JS bundle exists in
  the embedded FS (i.e. there's a file matching `_app/immutable/*.js`
  or whatever the Svelte build emits — discover at implementation
  time). Either assertion alone is weaker: `index.html` could be the
  stub the reporter actually got; one JS file without `index.html` is
  unusable. Both together prove a real build.
- **Run the guard at the right point in the pipeline.** After
  `make frontend-build` (so the FS is populated) and BEFORE
  `go build` (so the matrix doesn't spend time building binaries
  destined to be rejected). Either as a step inside the matrix
  `build` job (right before `go build`) or as a separate job that the
  matrix `needs`. Inside the matrix is simpler; gate to
  `matrix.binary == 'portal'`.
- **Belt-and-suspenders bash check after `go build`.** After the
  portal binary is built, add `strings dist/portal-* | grep -q
  "<!doctype html>"` to the build step. Cheap, catches a different
  failure mode (binary built OK but the embed somehow stripped or
  truncated — vanishingly rare but the check costs ~50ms).

## Acceptance criteria

- [ ] New file `internal/portal/assets/assets_release_test.go` (or
      similar; the build-tag-gated file separation determines the
      filename pattern) with `//go:build release` at the top. Contains
      one test function that:
      1. Calls into the package's embedded FS (use the existing `dist`
         var or `Handler()` — pick whichever exposes the FS cleanly;
         may need to export a small accessor for testing or use
         package-internal access).
      2. Asserts `dist/index.html` exists and is non-empty.
      3. Asserts at least one file matching the Svelte build's JS
         bundle pattern exists (discover the pattern at implementation
         — likely `_app/immutable/chunks/*.js` or similar; if Svelte
         emits to a different path, match what's actually there).
      4. Test name: `TestAssets_EmbeddedSPAIsBuilt` or similar.
      5. Failure messages name what was missing so a CI failure log
         tells the maintainer exactly what's wrong ("expected
         dist/index.html to be embedded but the file was not found —
         did make frontend-build run before go build?").
- [ ] `go test ./...` (no tags) on a fresh checkout passes — the new
      test must NOT run by default. Verify by:
      1. `git stash` any local frontend build state
      2. `go test ./...` — passes (the release-tagged test doesn't run)
      3. Restore working state
- [ ] `go test -tags release ./internal/portal/assets/...` runs the
      new test:
      - With `make frontend-build` already run → test PASSES.
      - With `dist/` containing only `.gitkeep` (simulate by removing
        `dist/index.html` temporarily) → test FAILS with the named
        error message.
- [ ] `.github/workflows/release.yml` `build` job (matrix entry where
      `matrix.binary == 'portal'`): add a step BEFORE `go build` that
      runs `go test -tags release ./internal/portal/assets/...`. Gate
      on the portal matrix so the jamsesh matrix entries skip cleanly.
- [ ] `.github/workflows/release.yml` `build` job (same gate): add a
      belt-and-suspenders post-build step that runs
      `strings dist/portal-${{ matrix.target.goos }}-${{ matrix.target.goarch }}* | grep -q "<!doctype html>"`
      and fails with a clear `::error::` annotation if the embed is
      missing. Cheap, catches a different failure mode than the Go
      test.
- [ ] Both new release.yml steps include an explanatory comment
      naming this story / the v0.1.1 incident, so future maintainers
      reading the workflow understand why the guard exists.
- [ ] Verify in a `workflow_dispatch` dry-run that both the Go test
      step and the post-build strings check actually fire and would
      catch a regression. Document in implementation notes.

## Reproducer (the regression this guard catches)

See sibling `bug-release-workflow-missing-frontend-build` for the
end-to-end reproducer. In short: without `make frontend-build` before
`go build`, the embedded SPA is empty, and the published image's
home page is a stub. This story's test catches that state at the
post-frontend-build / pre-go-build checkpoint in CI.

## References

- The bug this guard catches: see
  `bug-release-workflow-missing-frontend-build`.
- Embed code: `internal/portal/assets/assets.go` —
  `//go:embed all:dist` at line 21,
  `Handler()` accessor at lines 27-50.
- Current dist/ contents on a fresh checkout:
  `internal/portal/assets/dist/.gitkeep` (placeholder so embed doesn't
  fail to compile).
- Workflow file the guard wires into:
  `.github/workflows/release.yml:46-75`.

## Notes

- The build tag chosen (`release`) is a convention this story
  introduces — it's not used elsewhere yet. If the project later
  wants a different tag name (e.g., `ci`, `embed_required`), audit
  this story's test file and the release.yml step together. Pick the
  name that reads cleanly at the call site: `go test -tags release`
  is clear.
- An alternative to the build tag is an env-var gate inside the test
  (`if os.Getenv("EMBED_REQUIRED") == "" { t.Skip(...) }`). The
  build-tag approach is preferred because it's static (the test is
  literally absent from the binary when the tag isn't set), but the
  env-var approach is OK if the build tag turns out to interact
  badly with `go vet` or other tools. Decide at implementation.
- This story is the regression guard for ONE specific bug. Broader
  release-pipeline test coverage (binary smoke, container smoke,
  signature verification) is a separate epic. Don't expand scope.

## Out of scope

- **A full release-pipeline e2e test** (pulling the built image,
  running it, hitting `/`, asserting the SPA shell loads). Useful
  but much bigger. Park as a separate story under a future
  `epic-release-pipeline-hardening` or similar.
- **A pre-commit hook that runs the guard locally.** Local dev
  doesn't need this — the guard is for the release pipeline.
- **Generalizing the guard to assert specific bundle hashes or
  content shape.** Brittle and doesn't add value over "the file
  exists and is non-empty." Out of scope.
