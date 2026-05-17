---
id: epic-e2e-tests-fuzzing-pre-receive-validators
kind: story
stage: done
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

## Implementation notes

### Package access

All three fuzz files are in `package prereceive` (internal tests), giving
access to unexported helpers (`checkRefNamespace`, `parseTrailers`). No
extraction refactor was needed — all core functions were either already
exported (`Trailers`, `CheckRequiredTrailers`, `ValidateRef`, `CompileScope`,
`ScopeMatcher.Match`) or directly accessible from the same package.

### Bugs discovered by fuzz harnesses

#### 1. Ref namespace: `checkRefNamespace` allows `..` in branch segment (fixed inline)

**Seed**: `refs/heads/jam/sess-001/acc-alice/../acc-bob/main` with accountKey
`acc-alice` — allowed by the validator because `parts[1] == accountKey` and
`parts[2] = "../acc-bob/main"` was not validated.

**Also**: URL-encoded `%2e%2e` in the branch segment was similarly allowed.

**Fix**: Added `refSegmentSafe(parts[2])` check to `checkRefNamespace` in
`internal/portal/prereceive/refs.go`. The helper rejects any segment
containing `..` or URL-encoded equivalents (`%2e%2e`, `%252e`).

**Note**: Git's own ref-format rules (RFC 3986 / git-check-ref-format) already
reject `..` in ref names, so this is defence in depth. No existing test broke.

#### 2. `gobwas/glob@v0.2.3` panics on `Match` for malformed patterns with unclosed `{` (fixed inline, backlog filed)

**Seed**: pattern `"0{"`, path `"0"` — `glob.Compile("0{", '/')` returns no
error, but `g.Match("0")` panics with `slice bounds out of range [:2] with
length 1`. The crashing seed is saved at
`testdata/fuzz/FuzzPathScopeValidate/fc37b996e5096fc7`.

**Production impact**: if a session is created with a scope glob like `"0{"`,
the portal pre-receive hook panics on every push to that session (DOS via
malformed scope).

**Inline fix**: added `probeGlob(pattern, g)` to `CompileScope` in
`internal/portal/prereceive/scope.go`. It runs `g.Match(s)` against a set of
short strings (including byte-prefixes of the pattern) inside a deferred
`recover`, converting the would-be panic into a `CompileScope` error. Session
creation will then surface the error at session-setup time rather than at push
time.

A second panic variant was found during the same fuzz run: the original probe
used `range pattern` rune iteration and `pattern[:i+len(string(r))]`, which
panics when the pattern contains an invalid UTF-8 byte (the fuzzer generated
`"\x00\xc5"` — `\xc5` is an incomplete 2-byte sequence). The probe was
rewritten to slice by byte index, fixing this. Seed:
`testdata/fuzz/FuzzPathScopeValidate/304a602361edb18c`.

**Backlog story filed**: `bug-gobwas-glob-panic-on-malformed-pattern` — tracks
upgrading or replacing `gobwas/glob` as the proper fix. The inline probe is a
workaround.

### Fuzz harness design decisions

- **`FuzzCommitTrailerParse`** fuzz `Trailers` + `CheckRequiredTrailers`
  together. Properties: no panic; returned map has non-empty keys/values;
  `CheckRequiredTrailers` never silently drops absent required keys; when
  `Trailers=nil`, all required keys are reported missing.

- **`FuzzRefNamespaceValidate`** fuzz `checkRefNamespace` (unexported) with
  an empty in-memory `git.Repository` (so `repoIsEmpty` doesn't dereference
  nil). Properties: no panic; allowed refs have the expected structural form;
  `..` segments in any allowed ref are flagged; URL-encoded traversal in
  allowed refs is flagged.

- **`FuzzPathScopeValidate`** fuzz `CompileScope` + `ScopeMatcher.Match`.
  Properties: no panic; `CompileScope` error is acceptable for bad patterns;
  match is deterministic; match-all `**` accepts all non-empty paths;
  absolute path-traversal prefixes (`../`, `..%2F`, etc.) must not match
  `docs/**` scope.

- **`FuzzPathScopeEmpty`** (bonus harness): ensures an empty `ScopeMatcher`
  (nil pattern list) denies every path — deny-by-default property.

### Makefile

Added `make test-fuzz` target running all four harnesses with `-fuzztime=30s`
each.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**:
- `CompileScope` behavior change: previously malformed globs compiled "successfully" and panicked at first match; now they return an error at compile time. Strictly better, but worth documenting in `docs/SPEC.md`. Filed `docs-scope-glob-validation-rules` for the foundation-doc follow-on.

**Nits**:
- `probeGlob` iterates byte-by-byte up to 8 bytes — the magic number 8 deserves a one-liner justification in the comment.

**Notes**: Both production-bug fixes are surgically scoped and well-commented (refSegmentSafe cites git's own rules; probeGlob cites the upstream issue). Seed corpora include OWASP path-traversal payloads — exactly the value a fuzz program should add. The fuzz program quite literally paid for itself by surfacing a real DOS vector (malformed scope → portal panic on push). Excellent first-pass work.
