---
id: testing-release-spa-embed-guard
kind: story
stage: done
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

## Implementation notes

### New test file
`internal/portal/assets/assets_release_test.go`

Build constraint (first line of file): `//go:build release`

Package: `package assets` (same package as `assets.go`, enabling direct
access to the private `dist embed.FS` variable without exporting an accessor).

### Verification runs

**Pass case** (dist/ has built SPA — `assets/index-DnniF8dl.js` + `index.html`):
```
$ go test -tags release ./internal/portal/assets/... -v
=== RUN   TestAssets_EmbeddedSPAIsBuilt
    assets_release_test.go:59: embedded SPA verified: dist/index.html (391 bytes), found JS bundle at assets/index-DnniF8dl.js
--- PASS: TestAssets_EmbeddedSPAIsBuilt (0.00s)
PASS
ok  	jamsesh/internal/portal/assets	0.002s
```

**Simulated regression** (dist/ cleared to only `.gitkeep`):
```
$ go test -tags release ./internal/portal/assets/... -v
=== RUN   TestAssets_EmbeddedSPAIsBuilt
    assets_release_test.go:28: expected dist/index.html to be embedded but it was not found — did `make frontend-build` run before `go build`? (err: open index.html: file does not exist)
--- FAIL: TestAssets_EmbeddedSPAIsBuilt (0.00s)
FAIL
FAIL	jamsesh/internal/portal/assets	0.002s
```

**Default `go test ./...` (no tag):**
```
?   	jamsesh/internal/portal/assets	[no test files]
```
The release-tagged file is absent from compilation — local dev and CI without
`-tags release` are unaffected. Full `go test ./...` suite: all 53 packages pass.

### release.yml edits

**Insertion A** — new step at line 57–65 (between the existing `build frontend`
step at line 53 and the `build` step at line 67). Runs
`go test -tags release ./internal/portal/assets/...`, gated on
`matrix.binary == 'portal'`, with a comment naming this story and the v0.1.1
incident.

**Insertion B** — post-build shell block at lines 98–107, inside the existing
`build` step's `run:` block immediately after the `go build` invocation.
Checks `strings "${out}" | grep -q "<!DOCTYPE html>"` — uses the exact case
that Vite emits in `index.html`.

Note: the design brief specified `<!doctype html>` (lowercase); the actual
Vite-emitted file uses `<!DOCTYPE html>` (uppercase). The grep was adjusted to
match the real output to avoid a false-positive miss.

### Deviations
- Belt-and-suspenders grep uses `<!DOCTYPE html>` (uppercase) not `<!doctype html>`
  (lowercase) as written in the design brief — Vite emits uppercase.
- No `stretchr/testify` used; plain `testing` package only, consistent with the
  file's simplicity.
- `go vet -tags release ./internal/portal/assets/...` and
  `go build -tags release ./internal/portal/assets/...`: both clean.

## Review (2026-05-19)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Sixty-line Go test with `//go:build release` tag, accessing
the package-private `dist embed.FS` via in-package access (clean, no
exports needed). Two-tier assertion: `index.html` exists + non-empty,
plus a JS-bundle walk that short-circuits on first match. Failure
messages name the missing artifact and point directly at
`make frontend-build` — exactly what a future maintainer hitting this
in CI logs needs. Twenty-one workflow lines: an `assert SPA is
embedded` step between `make frontend-build` and `go build` (gated to
portal), plus a `strings | grep -q "<!DOCTYPE html>"` belt-and-
suspenders check inside the build step's run block after `go build`.
Both new workflow steps carry substantive comments referencing the
v0.1.1 incident and this story id. **Notable deviation worth calling
out:** the design brief specified `<!doctype html>` (lowercase) for
the grep pattern; the agent verified the actual Vite output and
corrected it to `<!DOCTYPE html>` (uppercase). Good catch — would
have produced a false-positive miss in CI if implemented as designed.
Independent verification confirmed both gating modes:
`go test -tags release ./internal/portal/assets/...` → PASS (with
current dist/ populated from prior wave's local frontend-build);
`go test ./internal/portal/assets/...` (no tag) → `[no test files]`,
local dev unaffected. What's now possible: the assets.go contract
comment ("On every real build, `make frontend-build` runs before
`go build`, so dist/ is fully populated") is now both true (via the
sibling fix landed in commit `e58f84f`) AND enforced (via this guard,
landed in commit `f5659a2`). The class of "release ships an empty
SPA" bug is closed for good — any future regression to the build
ordering fails fast in CI before any `go build` cycles, and the
post-build strings check catches the rarer "embed stripped during
link" failure mode.
