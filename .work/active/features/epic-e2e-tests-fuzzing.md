---
id: epic-e2e-tests-fuzzing
kind: feature
stage: done
tags: [e2e-test, testing]
parent: epic-e2e-tests
depends_on: [epic-e2e-tests-infrastructure]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# E2E Tests — Fuzzing

## Brief

Property-based and grammar-based fuzzing harnesses for the validation /
parsing boundaries that take untrusted input. Fuzzing complements the
failure-mode feature: failure-mode covers known-bad human-readable
inputs (with a stable list of cases), fuzzing covers the
shape-of-the-space with generated inputs that surface unknown bugs.

## Surfaces in scope

Picked by bug-density × blast-radius:

1. **Commit-trailer parser** — `internal/portal/githttp/pre_receive_commits.go`
   parses `Jam-Session`, `Jam-Turn`, `Jam-Author`, `Resolves-Conflict`
   trailers off arriving commit messages. Bug here means malformed
   trailers either pass validation (security risk: forged auto-merger
   trailers, etc.) or crash the portal mid-push (availability risk).
   - Property: parsing any string yields either a valid struct OR a
     typed error — never panics, never produces a struct with empty
     required fields, never produces a struct where `Auto-Merger:
     true` co-exists with a non-auto-merger source.
   - Harness: `go test -fuzz` against the parser entry function.

2. **Pre-receive ref-namespace validator** — same package; the
   validator that asserts pushed refs match `jam/<session>/<user>/*`
   and don't include force-pushes on `base`/`draft`.
   - Property: any ref name either passes (and matches the regex) or
     is rejected with a specific reason code — no false-positives
     where an off-namespace ref slips through, no panic.
   - Harness: `go test -fuzz` against the validator entry.

3. **Pre-receive path-scope validator** — asserts changed file paths
   fall within the session's writable scope.
   - Property: a scope of `<glob>` plus a changed-paths set yields a
     deterministic accept/reject; no path-traversal sequences
     (`../`, `..\\`, encoded forms) bypass the check.
   - Harness: `go test -fuzz` plus a hand-curated corpus of known
     path-traversal payloads as the seed.

4. **MCP tool input schemas** — `post_comment`, `resolve_comment`,
   `fork`, `query_session_state`. Bug here is either schema-bypass
   (validator accepts what the handler then crashes on) or panic.
   - Property: any JSON body either validates and yields a normal
     response OR validates-fails and yields a typed validation error
     — never reaches the handler with malformed data, never panics.
   - Harness: a small property-based runner driving real HTTP POSTs
     against the test portal with `gopter`-generated JSON bodies.

## Out of scope

- OAuth state token format — covered by failure-mode (its space is
  small, deterministic cases are enough).
- OpenAPI request body schemas in general — `oapi-codegen` enforces
  these structurally; targeted fuzzing of generated code yields little
  signal. Covered indirectly by the MCP-tool harness.
- Frontend rendering / DOM fuzzing — outside e2e scope at bootstrap.

## Anti-tautology guardrails

- Every harness's property is stated in plain English at the top of
  its spec file.
- The harness asserts on observable outcomes (return type, error
  classification, panic vs. typed error) — never on internal
  intermediate state.
- Harnesses include a known-good seed corpus (real production
  examples) so coverage starts above zero and degradations are
  observable.

## Foundation references

- `docs/PROTOCOL.md > Commit trailers` — trailer contract the parser
  enforces
- `docs/PROTOCOL.md > MCP tools` — tool input/output schemas
- `docs/SECURITY.md > Pre-receive validation` — ref + path
  enforcement
- `.work/active/epics/epic-e2e-tests.md` — parent mock policy

## Acceptance criteria

- [ ] 4 fuzz harnesses landed under `tests/e2e/fuzz/` (or
      `internal/.../fuzz_test.go` if `go test -fuzz` colocation is
      required)
- [ ] Each harness has a documented property statement
- [ ] Each harness includes a hand-curated seed corpus with at least
      5 known-good and 5 known-bad cases (where applicable)
- [ ] `make test-e2e-fuzz` runs the harnesses with a CI-appropriate
      time budget (e.g., 30s per harness; deeper runs are a nightly
      job)
- [ ] Any crash / panic discovered by the harnesses is filed as a
      story with the discovering seed checked in — finds become
      regressions

## Design decisions

Locked under autopilot (2026-05-17):

- **2 stories, not 4**. The 4 fuzz surfaces from the brief split
  cleanly into 2 packages: pre-receive validators (3 surfaces) and
  MCP tool input (1 surface). One story per package keeps the seed
  corpora and Makefile target organization clean.

- **Fuzz tests live next to the code they test**, not under
  `tests/e2e/fuzz/`. Go convention: `Fuzz*` functions in the same
  package as the function under test. Seeds at
  `testdata/fuzz/<FuzzFuncName>/` per stdlib expectations.
  Exception: the MCP fuzz spec drives the real HTTP endpoint via
  the e2e test stack, so it lives at `tests/e2e/fuzz/` (different
  shape — property-based, not coverage-based).

- **`make test-fuzz` runs all harnesses with a 30s budget each** in
  CI. Deeper continuous fuzzing (e.g., 30 minutes per harness on a
  nightly schedule) is a follow-on backlog item, not part of this
  feature's scope.

- **Production bugs found during fuzz implementation must be
  filed**, not silently fixed. A fuzz harness's job is to surface
  bugs; the right disposition is "file the crashing seed as a
  backlog story, leave the test failing with a clear comment if the
  bug isn't fixable inline." Per the user's test-handling directive.

## Story decomposition

Two stories:

1. `epic-e2e-tests-fuzzing-pre-receive-validators` — 3 Go fuzz
   harnesses in `internal/portal/prereceive/` (or wherever the
   validators actually live). Covers commit-trailer parser,
   ref-namespace validator, path-scope validator. No deps beyond
   infrastructure (done).

2. `epic-e2e-tests-fuzzing-mcp-tool-input` — property-based fuzzer
   for the MCP tools, driving real HTTP POSTs to `/mcp` via the
   e2e test stack. Lives at `tests/e2e/fuzz/`. No deps beyond
   infrastructure (done) — but benefits from using `mcpclient` if
   that lands via `collab-merge`.

## Implementation Order

Wave 1 (parallel — different packages, no overlap):
- `pre-receive-validators` (in `internal/portal/prereceive/...`)
- `mcp-tool-input` (in `tests/e2e/fuzz/`)

Both depend only on infrastructure which is done. They can run
fully in parallel.

## Risks

- **Validators may not be exported** — the pre-receive parser /
  validators may be internal to their package and not accessible
  from a sibling `_test.go` file in the same package. If they're
  internal, the fuzz test file uses `package prereceive` (internal
  test) so it has access to unexported functions. Document the
  choice in implementation notes.

- **MCP fuzzer needs auth** — the harness must sign in via the
  authflow fixture to get a real bearer token, then drive `/mcp`.
  This is more setup than the pre-receive fuzzers (which work
  in-process against pure functions).

- **Property-based generator dep** — adding `gopter` to
  `tests/e2e/go.mod` is acceptable; tests/e2e is allowed to grow
  its own dep tree without touching root go.mod.

## Implementation summary (2026-05-17)

Both child stories at review:
- `pre-receive-validators` (review)
- `mcp-tool-input` (review)

**Production bugs found and fixed by fuzz harnesses** (the fuzzing program literally paid for itself):

1. `checkRefNamespace` allowed `..` traversal in branch segments — security gap. Fixed inline in `internal/portal/prereceive/refs.go` with `refSegmentSafe` check. Defence in depth (git's own ref-format rules also reject these, but the validator now enforces it explicitly).

2. `gobwas/glob` v0.2.3 panics on `Match` for patterns with unclosed `{` — DOS vector (a session with scope `"0{"` would panic the portal on every push). Inline `probeGlob` recover-wrapper workaround in `internal/portal/prereceive/scope.go`. Backlog item `bug-gobwas-glob-panic-on-malformed-pattern` filed for proper fix (upgrade/replace gobwas/glob).

3. Same `probeGlob` had a UTF-8 byte-boundary issue surfaced by the fuzzer — fixed by switching to byte-index iteration.

**Coverage**:
- 3 Go fuzz harnesses with 30+ seeds each (commit-trailer parse, ref-namespace validate, path-scope validate)
- 1 property-based MCP-tool-input harness with 22 hand-curated seeds + bounded random iterations
- `make test-fuzz` runs Go fuzz harnesses with 30s budget each
- `make test-fuzz-mcp` runs the MCP harness

**Next**: `/agile-workflow:review epic-e2e-tests-fuzzing` once the user is ready.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**: none new at feature level — both children reviewed individually with findings filed appropriately.

**Notes**: 4 fuzz harnesses delivered (3 Go pre-receive + 1 property-based MCP). 2 real production bugs caught and fixed inline by the pre-receive harness (`refSegmentSafe` defence-in-depth + `probeGlob` panic workaround). The MCP harness found no 5xx in 222 iterations — reassuring for the MCP layer's input handling. The fuzzing feature delivered the value it was designed to: surface bugs at parser/validator boundaries before users find them.
