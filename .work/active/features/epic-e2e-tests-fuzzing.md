---
id: epic-e2e-tests-fuzzing
kind: feature
stage: drafting
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
