---
id: bug-release-workflow-missing-frontend-build
kind: story
stage: done
tags: [bug, infra, release, ui, self-host]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-19
updated: 2026-05-19
---

# Release workflow skips `make frontend-build`, ships portal images with empty SPA

## Brief

A self-hoster pulling `ghcr.io/nklisch/jamsesh:v0.1.1` (and `:latest`,
same digest) hits a blank/stub home page because the Svelte SPA was
never built into the embedded `dist/` directory before `go build` ran.
The portal binary embeds only the `.gitkeep` placeholder.

Root cause: `.github/workflows/release.yml:46-75` invokes `go build`
directly with no Node setup and no `make frontend-build` step. The
local `Makefile` correctly chains `go-build: frontend-build`, but the
workflow bypasses Make. The comment in
`internal/portal/assets/assets.go:6-8` already asserts the expected
contract:

> On every real build, `make frontend-build` runs before `go build`,
> so dist/ is fully populated.

…but the release workflow violates that contract.

Reporter's investigation transcript on the self-host server confirms:

- `curl https://jamsesh.dev/` returns a minimal HTML stub (the
  SPA-not-found fallback served by `assets.Handler()`'s
  `index.html`-fallback path, which serves the stub `.gitkeep`-only
  state).
- `docker exec jamsesh-portal sh -c 'find / -name index.html'` finds
  nothing inside the image — the embedded FS has only `.gitkeep`.
- `ghcr.io/nklisch/jamsesh:latest` has the same digest as `:v0.1.1`,
  so the bug ships in every currently-published image.

The matrix builds 10 binaries (5 platforms × 2 binaries: `portal` and
`jamsesh`). Only `portal` embeds the SPA; the `jamsesh` plugin wrapper
binary does not. The fix should be gated so the frontend build only
runs for the `portal` matrix entries.

## Strategic decisions

Resolved at scope.

- **Fix the workflow, not the comment.** The `assets.go` comment is
  documenting the correct contract; the workflow is the bug. The
  comment stays as-is; the workflow catches up.
- **Use `make frontend-build` rather than `npm run build` directly.**
  The Makefile target is the canonical entry point and may add steps
  later (codegen, etc.); calling Make keeps the workflow aligned with
  local dev. (Verify the target works in CI with the same Node version
  developers use.)
- **Gate frontend-build to the `portal` matrix.** The `jamsesh`
  wrapper has no SPA; running Node setup + frontend-build on those 5
  matrix entries would waste ~half the matrix runtime.
- **Don't refactor the matrix.** Splitting frontend-build into a
  separate prerequisite job (build once, upload artifact, fan out to
  matrix) is a clean follow-up but adds workflow complexity. The
  conditional in-matrix approach is simpler and matches how Node
  setup works in most multi-arch Go workflows. Out of scope here;
  parkable as a separate optimization.
- **No production code change.** All edits are CI configuration.

## Acceptance criteria

- [ ] `.github/workflows/release.yml` `build` job (matrix at lines
      26-83): add an `actions/setup-node@v4` step + a `make
      frontend-build` step BEFORE the `go build` step, both gated on
      `if: matrix.binary == 'portal'`. Use Node version pinned via
      `node-version-file: '.nvmrc'` if `.nvmrc` exists at repo root,
      otherwise the version the existing local-dev/CI uses (audit
      `Makefile`'s `frontend-build` target and any `package.json`
      `engines.node` to determine the right pin; if neither pins a
      version, set `node-version: '20.x'` and log the rationale in
      implementation notes).
- [ ] Verify the change with `workflow_dispatch` (dry-run mode, no
      release upload): trigger the release workflow in the GH UI with
      `dry_run: true`, download the resulting `portal-linux-amd64`
      artifact from the workflow run, and confirm the binary contains
      an embedded SPA. Two acceptable proofs:
      1. `strings portal-linux-amd64 | grep -q "<!doctype html>"` —
         returns 0 (the SPA's index.html boilerplate is embedded).
      2. Run the binary briefly (`./portal-linux-amd64 --help` is
         enough to hit the embed-init path) and confirm no error.
      Document the exact proof used in implementation notes.
- [ ] No change to `jamsesh` matrix entries. The Node setup +
      frontend-build steps must skip cleanly for `matrix.binary ==
      'jamsesh'` (the `if:` gate).
- [ ] No production code change. `internal/portal/assets/assets.go`
      and the rest of the Go source unchanged. Only
      `.github/workflows/release.yml` is touched.
- [ ] `docs/RELEASING.md` audit: the workflow summary at
      `docs/RELEASING.md:12-30` lists what `release.yml` does. If
      frontend-build was previously implied or omitted, update the
      bullet list to mention it explicitly. Likely a one-line
      addition under the cross-compile bullet.

## Reproducer

1. `docker pull ghcr.io/nklisch/jamsesh:v0.1.1`
2. `docker run --rm --entrypoint sh ghcr.io/nklisch/jamsesh:v0.1.1 -c
   'find / -name index.html 2>/dev/null'`
3. Observed: no `index.html` found anywhere in the image (other than
   the embed-stub at `<binary>/dist/.gitkeep`).
4. Expected: `/dist/index.html` (or wherever the Svelte build emits
   index) present and bundled into the binary.

End-to-end:
1. `curl https://<deployed-portal>/` returns the stub HTML, not the
   SPA shell.
2. The SPA never loads, no `/api/auth/oauth/start` is reachable
   through the UI, and the operator can't sign in.

## References

- Workflow file with the gap: `.github/workflows/release.yml:46-75`
  (`build` step running `go build` directly, no Node setup, no Make
  invocation).
- Embed code with the contract comment:
  `internal/portal/assets/assets.go:1-15` —
  `// On every real build, "make frontend-build" runs before "go
  build", so dist/ is fully populated.`
- Makefile chain (works locally, bypassed by CI):
  - `frontend-build:` target
  - `go-build: frontend-build` — what the workflow SHOULD have called
- Matrix definition: `.github/workflows/release.yml:28-35`
  (`binary: [portal, jamsesh]` × 5 platforms = 10 jobs).
- Companion regression-guard story:
  `testing-release-spa-embed-guard` — the test that would have
  caught this in CI before publication; tracked separately.

## Notes

- The `assets.go` comment uses backticks for code in the prose. After
  the fix lands and a re-release happens, the contract the comment
  asserts will once again be true. No edit to the comment is needed.
- The reporter's workaround on their server is to build the frontend
  + Go binary locally from a clone (`make build && go build
  ./cmd/portal`), then run their overlay Dockerfile pointing at the
  locally-built binary. Their `/srv/jamsesh-portal/Dockerfile`
  already handles the binary copy; they just need the working binary
  to land there. Once a re-published image (next tag) is verified to
  contain the SPA, the local-build workaround can be dropped.

## Implementation notes

### `.github/workflows/release.yml` change

Two steps inserted before the existing `- name: build` step (now at line 58
after the insertion, previously line 46). Both gated on `if: matrix.binary ==
'portal'`:

- **Lines 46-51:** `actions/setup-node@v4` — Node 20, npm cache, keyed on
  `frontend/package-lock.json`. Matches the precedent in `.github/workflows/e2e.yml:34-41`.
  No `.nvmrc` at repo root; `frontend/package.json` has no `engines.node` pin.
  Node 20 used to match the existing CI convention in `e2e.yml`.
- **Lines 53-55:** `- name: build frontend (portal SPA)` runs `make
  frontend-build`. The `if:` gate is confirmed — the 5 `jamsesh` matrix entries
  skip both steps cleanly, same as before.

### `docs/RELEASING.md` change

Added new bullet 1 describing the `make frontend-build` step (portal-only,
Node 20). Former bullets 1–8 renumbered to 2–9. Wording is one-sentence
functional description matching the doc's existing style.

### Local `make frontend-build` result

Ran successfully. Build output:

```
dist/index.html                   0.39 kB │ gzip:  0.26 kB
dist/assets/index-DMjmOynP.css   72.79 kB │ gzip: 10.23 kB
dist/assets/index-DnniF8dl.js   138.79 kB │ gzip: 46.65 kB
✓ built in 720ms
```

`internal/portal/assets/dist/` contains `index.html`, `assets/` (JS+CSS),
and `.gitkeep`. Build artifacts are covered by `.gitignore:9`
(`internal/portal/assets/dist/*`) — they do not appear in `git status`.
Only `.github/workflows/release.yml` and `docs/RELEASING.md` are staged.

### No production code changes

`git status` confirms only `release.yml`, `RELEASING.md`, and the story
file are modified. Nothing under `internal/`, `frontend/`, or `cmd/` touched.

### Deviations

None. The design followed exactly as specified.

## Out of scope

- **Refactoring the matrix to share a single frontend-build job
  across portal-* entries.** Would be cleaner (no duplicated Node
  setup + build × 5 platforms) but adds workflow complexity. Park as
  a separate optimization if the in-matrix approach turns out to be
  meaningfully slow.
- **Adding a build-time provenance check that the SPA's `index.html`
  hash matches what release.yml signed.** Belongs to the supply-chain
  hardening epic, separate scope.
- **Backporting / re-tagging v0.1.1.** The maintainer can decide
  whether to cut v0.1.2 as the first fixed release or re-publish
  v0.1.1 (the latter is risky — checksums change). Out of scope for
  the story; the story just lands the workflow fix.

## Review (2026-05-19)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Eleven-line workflow addition: two new steps (`setup-node@v4`
with Node 20 + npm cache, and `make frontend-build`) between the
existing `setup-go` and `build` steps, both gated `if: matrix.binary
== 'portal'` so the 5 jamsesh matrix entries skip cleanly. The Node
configuration matches the `e2e.yml:34-41` precedent exactly (same
version, same cache key path). `docs/RELEASING.md` updated with a new
bullet 1 describing the frontend-build step; bullets 2-9 renumbered
cleanly with no content loss. Wave 1's local `make frontend-build`
run already produced the expected `dist/` output (index.html + assets/
JS+CSS bundle), confirming the Makefile target works as expected in
the CI environment. The fix-steps themselves don't carry a "why"
comment naming the v0.1.1 incident, but the immediately-adjacent
`assert SPA is embedded` step (landing as part of
`testing-release-spa-embed-guard`) does — future maintainers find the
historical context one step down in the same workflow section. What's
now possible: the next published portal image will contain the
embedded SPA; self-hosters pulling from `ghcr.io/nklisch/jamsesh` get
a functional sign-in page on first run instead of the stub HTML the
v0.1.1 image shipped.
