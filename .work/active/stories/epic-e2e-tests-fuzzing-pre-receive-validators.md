---
id: epic-e2e-tests-fuzzing-pre-receive-validators
kind: story
stage: implementing
tags: [e2e-test, testing, portal]
parent: epic-e2e-tests-fuzzing
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Fuzzing — Pre-receive validator harnesses

## Scope

Three Go fuzz harnesses for the pre-receive pipeline's parsers /
validators, colocated with the code under test in
`internal/portal/prereceive/` (or `internal/portal/githttp/` —
inspect the codebase to find the exact package).

Each harness uses Go's stdlib `go test -fuzz` discovery. Seed corpora
live under `testdata/fuzz/<FuzzFuncName>/` per Go convention.

## Surfaces

1. **Commit-trailer parser** — function that extracts
   `Jam-Session`, `Jam-Turn`, `Jam-Author`, `Resolves-Conflict`,
   `Auto-Merger`, `Source-Commit` trailers from a commit message
   body. Invariant: any input either yields a typed result OR a
   typed error — never panics, never produces a result with empty
   required fields, never accepts `Auto-Merger: true` alongside a
   non-auto-merger source.

2. **Ref-namespace validator** — function that asserts pushed refs
   match `jam/<session>/<user>/*` and rejects force-pushes on
   `base`/`draft`. Invariant: any ref name either passes (and
   matches the documented regex) or is rejected with a specific
   reason code — no false-positives where an off-namespace ref
   slips through, no panic.

3. **Path-scope validator** — function that asserts changed file
   paths fall within the session's writable scope (declared as a
   glob). Invariant: a glob plus a path set yields a deterministic
   accept/reject; no path-traversal sequences (`../`, `..\\`,
   URL-encoded forms, double-encoded forms) bypass the check.

## Files to create / modify

- `internal/portal/prereceive/commits_fuzz_test.go` (NEW) — `FuzzCommitTrailerParse`
- `internal/portal/prereceive/refs_fuzz_test.go` (NEW) — `FuzzRefNamespaceValidate`
- `internal/portal/prereceive/paths_fuzz_test.go` (NEW) — `FuzzPathScopeValidate`
- `internal/portal/prereceive/testdata/fuzz/FuzzCommitTrailerParse/` — seed corpus
- `internal/portal/prereceive/testdata/fuzz/FuzzRefNamespaceValidate/` — seed corpus
- `internal/portal/prereceive/testdata/fuzz/FuzzPathScopeValidate/` — seed corpus
- `Makefile` — new target `test-fuzz` that runs `go test -fuzz=Fuzz -fuzztime=30s ./internal/portal/prereceive/...`

(If the validators are in a different package than `prereceive`, adapt
paths accordingly. Run `grep` for the function signatures first.)

## Acceptance criteria

- [ ] All three fuzz harnesses run green with no panics under
      `go test -fuzz=. -fuzztime=10s ./internal/portal/prereceive/...`
- [ ] Each harness has at least 5 known-good and 5 known-bad seeds
      checked into `testdata/fuzz/<name>/`
- [ ] The path-scope seed corpus explicitly includes path-traversal
      payloads (`../etc/passwd`, `..%2F`, `..\\`, `%2e%2e/`) — these
      must NOT bypass the validator
- [ ] `make test-fuzz` runs all three harnesses with a 30s budget
      each in CI
- [ ] Any crash discovered by the harnesses during implementation
      is filed as a story with the discovering seed checked in
      (production-bug filing per the user's test-handling directive)
- [ ] Each harness's property is stated in plain English at the top
      of its `Fuzz*` function doc comment

## Notes for the implementer

- Per the user directive: if you find a real production bug
  (panic, false-negative, false-positive), file it as a backlog
  story with `tags: [bug]` and don't try to "make the test pass" by
  weakening the assertion. It's fine for a fuzz harness to surface
  a real bug — that's its job.
- The validator functions should be EXPORTED or available to the
  same package. If they're tightly coupled to HTTP handlers, you
  may need a thin extraction — keep the extraction minimal and
  document it in the implementation notes.
- Seed corpora: gather real production commit messages from
  `git log` and known attack patterns from OWASP's path-traversal
  cheat sheet (representative examples — not an exhaustive list).
- Use `t.Fatalf("panic on input %q: %v", input, r)` with `recover()`
  in the fuzz function so panics produce readable seeds. Actually
  this is unnecessary — Go's fuzz runner converts panics into
  failures automatically and saves the seed.

## Risks

- The validators may not be currently exported. The implementor
  may need to either export them or add a thin internal API for
  the fuzzer. Document the decision in implementation notes.
- Path-traversal coverage is partial without filesystem-level
  semantics — the validator works on logical paths, but a real
  attack might exploit OS path canonicalization. This harness
  surfaces logical-path bypasses; OS-level testing would be a
  separate concern.
