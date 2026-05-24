---
id: feature-playground-server-hardening
kind: feature
stage: done
tags: [portal, playground]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# Playground server hardening

## Brief

Three review-surfaced server-side issues in `internal/portal/playground/`
bundled into one feature for a shared design pass. All three were
filed during review of
`story-epic-ephemeral-playground-session-lifecycle-rest-endpoints`
(rest endpoints) and the parent feature `feature-epic-ephemeral-playground-session-lifecycle`.

## Why a feature

The three children share a code area and one of them carries a
cross-cutting refactor decision worth a single design pass:
extract `validateWritableScope` from `internal/portal/sessions/`
into a shared package (proposed: `internal/portal/sessionscope/`)
importable from both the durable session handler and the playground
handler. Bundling under one feature gives the work a coherent verdict
and a clean PR shape.

## Child stories

- `story-playground-server-hardening-wordlist-dedup` — dedupe 62
  duplicate adjectives in
  `internal/portal/playground/wordlist/adjectives.txt`
- `story-playground-server-hardening-handler-test-coverage` — add 3
  missing handler tests + refactor `openStore(t)` into a `stores(t)`
  helper so every test exercises both SQLite and Postgres
- `story-playground-server-hardening-writable-scope-validation` —
  validate the `Scope` field on `CreatePlaygroundSession` and extract
  `validateWritableScope` into a shared package

## Design notes (for /agile-workflow:feature-design)

The interesting design decision is the home of the extracted
`validateWritableScope` helper. Candidates:

1. New `internal/portal/sessionscope/` package — clean separation,
   imported by both `sessions` and `playground`
2. Move into `internal/portal/storage/` alongside other shared
   primitives
3. Promote to a higher-level helper inside an existing shared package

Option 1 looks correct on the surface (single-responsibility, named
for what it protects). `feature-design` should confirm by checking
what else might want to live in such a package (e.g., scope parsing,
scope normalization).

The handler-test-coverage story also lands a cross-cutting test
refactor (`openStore(t)` → `stores(t)`) — feature-design should
sequence it after the validation story so the new tests for
validation use the dialect-aware harness from the start.

## Design decisions

- **Home of extracted `validateWritableScope`**: extend `internal/portal/prereceive/` — that package already owns `CompileScope` (exported) and a private `parseWritableScope` (parse-only, no glob). Consolidating all scope-related primitives there avoids a new single-function package, accepts a mild responsibility blur (prereceive becomes both push-time validator and write-time validation provider), and gives sessions + playground a single import edge.
- **`stores(t)` test helper**: consolidate into a shared test-helper package — canonical `stores(t)` (currently in `internal/db/store/helpers_test.go` with truncateAll cleanup) moves to a shared location (e.g. `internal/db/store/testharness/` or a test-tagged file in the store package), so the existing duplicate in `internal/portal/playground/provision_test.go` and the new 3rd usage in `playground/handler_test.go` import one source. The drift risk (one version has cleanup, the other doesn't) gets fixed in the same pass.
- **Implementation order**: stores(t) refactor first → writable-scope-validation → wordlist-dedup (parallel-safe). The handler-test-coverage story delivers the shared `stores(t)` helper as part of its work; the validation story's new tests then use the dialect-aware harness from the start and get Postgres coverage for free. This reverses the literal reading of the original feature body — feature-design confirms the intent was "harness first" so downstream tests inherit it.

## Acceptance (rollup)

- All three child stories at stage:done with verdicts ≥ approve-with-comments
- `validateWritableScope` lives in `internal/portal/prereceive/` and is imported
  by both `internal/portal/sessions/` and `internal/portal/playground/`
- `stores(t)` is consolidated into one location and consumed by sessions tests,
  playground provision tests, and playground handler tests
- `go test ./internal/portal/playground/...` passes against both
  SQLite and Postgres
- `sort internal/portal/playground/wordlist/adjectives.txt | uniq -c | awk '$1>1'`
  returns no rows

## Architectural choice

Three independent changes that share a code area but split cleanly along
file boundaries:

1. **Test harness consolidation (`stores(t)`)** — Land this first as a
   pure refactor. The current duplicate lives across three `_test`
   packages (`store_test` in `internal/db/store/helpers_test.go`,
   `playground_test` in `internal/portal/playground/provision_test.go`,
   plus a divergent local `openStore(t)` in
   `internal/portal/playground/handler_test.go` and another in
   `internal/portal/sessions/handler_test.go`). They can't import each
   other because all are `_test` packages. The fix: extract a new
   importable package `internal/db/store/storetest` (no `_test` suffix)
   that exports `Stores(*testing.T) []DialectHarness`. Each call site
   then delegates a one-line wrapper. We adopt the truncate-on-cleanup
   behavior (currently in `store_test`) as canonical — the playground
   version's lack of Postgres truncate is a latent bug for shared-schema
   isolation. Considered alternatives — a `//go:build testharness` file
   inside `store` or duplicating across each test package — both lose
   the cross-package consumption that's the whole point.

2. **`validateWritableScope` extraction** — Move into
   `internal/portal/prereceive/` as the exported function
   `ValidateWritableScope(raw string) (msg string, ok bool)`. The
   package already owns `CompileScope` (export) and `parseWritableScope`
   (unexported helper inside `validate.go`); the new function is
   `parseWritableScope` + `CompileScope` glued together with the
   error-as-message shape the handlers want. We also rename the existing
   unexported `parseWritableScope` to keep the original behavior intact
   — no breaking change to internal callers. Considered alternatives
   — new `sessionscope` package or living in `storage` — were rejected
   in `## Design decisions`. The mild responsibility blur (prereceive
   becomes both push-time policy and write-time validation provider) is
   intentional: scope semantics belong together.

3. **Wordlist dedup** — Pure data change on
   `internal/portal/playground/wordlist/adjectives.txt`. Independent of
   the other two units; sequenced last so it parallel-runs cleanly.

## Implementation Units

### Unit 1: Shared test harness package

**File**: `internal/db/store/storetest/storetest.go` (new)
**Story**: `story-playground-server-hardening-handler-test-coverage`

```go
// Package storetest provides cross-dialect store fixtures usable from any
// _test package. SQLite is always available; Postgres is included only
// when JAMSESH_TEST_PG_DSN is set so local iteration stays fast.
package storetest

import (
    "context"
    "os"
    "testing"

    "github.com/jackc/pgx/v5/pgxpool"
    "github.com/jackc/pgx/v5/stdlib"

    "jamsesh/internal/db"
    "jamsesh/internal/db/store"
)

// DialectHarness names a SQL dialect and an opener that returns a fresh
// store with t.Cleanup wired (close + truncate for postgres).
type DialectHarness struct {
    Name string
    Open func(t *testing.T) store.Store
}

// Stores returns one harness per available dialect.
func Stores(t *testing.T) []DialectHarness { ... }

// truncateAll clears all tables in dependency-safe order via CASCADE.
// Exported only as an implementation detail of the postgres harness'
// cleanup — not part of the public surface.
func truncateAll(t *testing.T, dsn string) { ... }
```

**Implementation Notes**:
- Body is a near-verbatim port of `internal/db/store/helpers_test.go:33-108`
  (the existing canonical version with Postgres truncate). Field names
  capitalize: `name → Name`, `open → Open`.
- Keep `Stores` exported as the only public function. `DialectHarness`
  exported so callers can range over the return value with named fields.
- No dependency on `_test` build tag — this is regular Go code that
  happens to take `*testing.T`. The standard pattern (see
  `tests/e2e/fixtures/`).

**Call-site updates**:
- `internal/db/store/helpers_test.go`: delete the local `stores` and
  `truncateAll`. Add a one-line wrapper: `func stores(t *testing.T)
  []storetest.DialectHarness { return storetest.Stores(t) }` (or just
  inline `storetest.Stores(t)` at the existing call sites — both work).
- `internal/portal/playground/provision_test.go`: same — delete the
  local `dialectHarness` and `stores`, replace usages with
  `storetest.Stores(t)` (range loop reads `tt.Name`/`tt.Open`).
- `internal/portal/playground/handler_test.go`: replace
  `openStore(t)` (line 205) with a per-dialect loop using
  `storetest.Stores(t)` — see Unit 3 below for the test-coverage
  consequences.
- `internal/portal/sessions/handler_test.go`: replace local `openStore(t)`
  (line 219) similarly. This is an extra cleanup found during design —
  not in the original story body but mechanically cheap once the harness
  exists. Treat as part of the same story.

**Acceptance Criteria**:
- [ ] `internal/db/store/storetest/storetest.go` exists with the
  documented public surface.
- [ ] Zero duplicate copies of the `stores(t)` helper or `dialectHarness`
  type remain in the repo (`grep -rn 'func stores(t \*testing.T)' .`
  returns only the wrapper in `helpers_test.go`).
- [ ] All test files that previously called `openStore(t)` in
  `playground` and `sessions` packages now consume `storetest.Stores(t)`.
- [ ] `go test ./internal/db/store/... ./internal/portal/playground/...
  ./internal/portal/sessions/...` passes (SQLite-only locally).
- [ ] With `JAMSESH_TEST_PG_DSN` set, the same command exercises both
  dialects and Postgres rows are truncated between tests.

---

### Unit 2: `ValidateWritableScope` in `prereceive`, called by playground create

**File**: `internal/portal/prereceive/scope.go` (extend, add new exported func)
**File**: `internal/portal/playground/handler.go` (call site)
**File**: `internal/portal/sessions/handler.go` (replace local helper)
**Story**: `story-playground-server-hardening-writable-scope-validation`

```go
// Added to internal/portal/prereceive/scope.go:

// ValidateWritableScope parses the JSON-encoded writable_scope payload
// and compiles each glob through CompileScope. It returns ("", true) when
// the payload is acceptable (including the deny-all empty-string and "[]"
// cases), or (message, false) when the payload is unparseable JSON or
// contains a malformed glob. The message is suitable for the body of a
// session.invalid_writable_scope 400 envelope.
//
// Callers: sessions.CreateSession (front door), sessions.PatchSession
// (mutation), playground.CreatePlaygroundSession (front door). All three
// must give identical answers for identical inputs.
func ValidateWritableScope(raw string) (msg string, ok bool) {
    if raw == "" {
        return "", true
    }
    var globs []string
    if err := json.Unmarshal([]byte(raw), &globs); err != nil {
        return fmt.Sprintf("writable_scope must be a JSON array of strings: %v", err), false
    }
    if _, err := CompileScope(globs); err != nil {
        return err.Error(), false
    }
    return "", true
}
```

**Implementation Notes**:
- The function body is a verbatim move from
  `internal/portal/sessions/handler.go:443-455` with the
  `prereceive.CompileScope` call shortened to local `CompileScope`.
- Add `encoding/json` and `fmt` to `scope.go`'s import list (currently
  only `fmt` + `strings` + doublestar).
- The internal `parseWritableScope` in `validate.go:86` stays as-is —
  same package, different signature (`([]string, error)`), still used
  by `Validate`. Do NOT rename it; the new export is conceptually a
  superset that goes one step further (compile-validate), and removing
  the parse-only helper would force `Validate` to discard its error
  context.
- Sessions handler: delete the local `validateWritableScope` (lines
  436-455), update the two call sites (lines 91, 217) to
  `prereceive.ValidateWritableScope(...)`.
- Playground handler: add the validation block BEFORE the existing
  scope-default fallback at `handler.go:98-101`. Specifically:

  ```go
  // Existing code:
  scope := strings.TrimSpace(body.Scope)
  if scope == "" {
      scope = `["**"]`
  }
  // NEW: validate after defaulting so the default "**" gets compile-checked
  // too (cheap insurance against a future default-value typo).
  if msg, ok := prereceive.ValidateWritableScope(scope); !ok {
      return openapi.CreatePlaygroundSession400JSONResponse(openapi.ErrorEnvelope{
          Error:   "session.invalid_writable_scope",
          Message: msg,
      }), nil
  }
  ```
- The error envelope code (`session.invalid_writable_scope`) matches the
  durable-session handler — clients can use one error-code branch for
  both surfaces.
- Confirm `openapi.CreatePlaygroundSession400JSONResponse` exists. If
  the generated types only have e.g. 503/410, the implementer must add
  `400` to the playground create operation in `docs/openapi.yaml` and
  regenerate via `go generate ./...`. (Inspection of `handler.go`
  doesn't show an existing 400 response on this op — likely needs the
  yaml change.)

**Acceptance Criteria**:
- [ ] `prereceive.ValidateWritableScope` is exported and documented;
  unit test in `internal/portal/prereceive/scope_test.go` covers
  (empty → ok), (`[]` → ok), (`["src/**"]` → ok), (`not json` → err
  with non-empty msg), (`["docs/{"]` → err with msg containing "bad
  pattern syntax").
- [ ] `internal/portal/sessions/handler.go` no longer defines a local
  `validateWritableScope`; both call sites delegate to
  `prereceive.ValidateWritableScope`.
- [ ] `POST /api/playground/sessions` with `scope: "not json"` returns
  400 `session.invalid_writable_scope` with a non-empty message.
- [ ] `POST /api/playground/sessions` with `scope: ["src/**"]` returns
  201 unchanged.
- [ ] Existing sessions tests (`scope_validation_test.go`) still pass
  unchanged — same envelope shape, same status codes.

---

### Unit 3: Playground handler test coverage

**File**: `internal/portal/playground/handler_test.go`
**Story**: `story-playground-server-hardening-handler-test-coverage`

Three new test cases plus a per-dialect refactor of the existing
`testEnv` builders so every test runs under both SQLite and Postgres.

```go
// Refactor newTestEnv to take a store from storetest.Stores(t):

func newTestEnv(t *testing.T, s store.Store, cfg playground.Config) *testEnv {
    t.Helper()
    return newTestEnvWithStore(t, s, cfg)  // existing builder, unchanged
}

// Every existing TestX becomes:
func TestX(t *testing.T) {
    for _, h := range storetest.Stores(t) {
        h := h
        t.Run(h.Name, func(t *testing.T) {
            env := newTestEnv(t, h.Open(t), defaultCfg())
            // ... existing body unchanged ...
        })
    }
}

// New: TestJoinPlaygroundSession_HardCapElapsed_Returns410
//   - Create session via store with HardCapAt = clock.Now() - 1*time.Hour
//   - Add a creator member row to satisfy the GetSession path
//   - POST /api/playground/sessions/{id}/join → assert 410 with
//     error="playground.session_ended"
//   - Covers branch handler.go:206-211 (HardCapAt elapsed).
//   - Also assert a second case where HardCapAt is in the future but
//     status is "ended" → 410 path handler.go:214-219.
//   - For the inner branch (handler.go:247-252 where ttl<=0 is checked
//     AFTER bearer issue), exercise via: HardCapAt = now+1ns; advance
//     fixedClock by 1 second between the GetSession call and the
//     ttl-check. The fixedClock as written returns t.t verbatim — for
//     this test only, swap to a stepClock that advances 1s per call so
//     the second clock read trips the ttl<=0 branch. Add stepClock
//     type to handler_test.go.

// New: TestCreatePlaygroundSession_RepoCreateFails_ReturnsError
//   - env.stor.createError = errors.New("disk full")
//   - POST /api/playground/sessions with empty body
//   - Assert: response is 500-class (httperr.WriteFromError will emit
//     a typed envelope from the wrapped error); the session row remains
//     in the store (orphaned, awaiting destruction sweep); the member
//     row was NOT added (CreateRepo runs after AddSessionMember in
//     current handler — verify this is still the order; if so the
//     member IS added before CreateRepo and we assert it remains too).
//   - Re-read handler.go:137-170: CreateRepo runs AFTER AddSessionMember.
//     So the orphaned state is { session row + creator member row + no
//     bare repo on disk }. The destruction sweep cleans by session_id.
//     Test asserts both rows persist via env.s.GetSession and
//     env.s.GetSessionMember.

// New (third bullet from story): the per-dialect refactor described
// above IS the third test-coverage item — it doesn't add a test
// function, it multiplies the existing test count by N dialects.
```

**Implementation Notes**:
- Drop the local `openStore(t)` (line 205) entirely. `newTestEnv` and
  `newTestEnvWithStore` both now take an explicit `store.Store` argument
  from the dialect loop.
- The `playgroundOnlyStrict` shim (lines 74-191) and `stubStorage`
  (lines 36-68) stay unchanged.
- `stepClock` is a new 3-line type:
  ```go
  type stepClock struct{ t time.Time; step time.Duration }
  func (c *stepClock) Now() time.Time { now := c.t; c.t = c.t.Add(c.step); return now }
  ```
  Use only in the hard-cap-elapsed inner-branch test; keep `fixedClock`
  for everything else.
- Test naming follows existing convention:
  `TestX_Condition_OutcomeY`. The three new tests fit the pattern.

**Acceptance Criteria**:
- [ ] Every `TestX` in `handler_test.go` runs as `TestX/sqlite` and (with
  `JAMSESH_TEST_PG_DSN` set) `TestX/postgres`.
- [ ] `TestJoinPlaygroundSession_HardCapElapsed_Returns410` covers both
  the outer `!Before(*HardCapAt)` branch and the inner `ttl<=0` branch
  via a stepping clock.
- [ ] `TestCreatePlaygroundSession_RepoCreateFails_ReturnsError` sets
  `stor.createError`, asserts the response is an error, and verifies
  both the session row and creator member row remain in the store.
- [ ] `go test ./internal/portal/playground/...` passes against both
  dialects.

---

### Unit 4: Wordlist dedup

**File**: `internal/portal/playground/wordlist/adjectives.txt`
**Story**: `story-playground-server-hardening-wordlist-dedup`

Pure data change. Sort the 239-line file, remove the 62 duplicates,
producing 177 unique entries. Optionally pad back up to ~256 unique
entries with additional curated calm/positive adjectives — at the
implementer's discretion; the acceptance is "no duplicates" and the
existing diversity test threshold (900/1000 distinct picks over the
joint adj×animal space) continues to pass.

**Implementation Notes**:
- The mechanical fix: `sort -u adjectives.txt > new && mv new adjectives.txt`.
- The diversity test (`wordlist_test.go::TestPick_Diversity`) needs the
  product `len(adjectives) * len(animals)` >> 1000 to clear 900 distinct
  picks. With 177 × 182 = 32214, easily clears the threshold.
- If padding back to 256, add words alphabetically interleaved so the
  diff stays reviewable; pull from the same calm/positive register
  already in the file (nature, weather, gentle states — "balmy",
  "luminous", "polished", "verdant", etc. — avoid charged adjectives).
- File ends with a trailing newline — preserve it.

**Acceptance Criteria**:
- [ ] `sort internal/portal/playground/wordlist/adjectives.txt | uniq -c | awk '$1>1'`
  returns no rows.
- [ ] `wc -l adjectives.txt` returns ≥ 177 (no regression in entry count).
- [ ] `go test ./internal/portal/playground/wordlist/...` passes.

---

## Implementation Order

1. **Unit 1** (`stores(t)` extraction to `storetest`) — story
   `story-playground-server-hardening-handler-test-coverage` lands the
   `storetest` package + the test-file rewrites for sessions/playground
   handler tests. No `depends_on`. Pure refactor — no behavior change.
2. **Unit 2** (`ValidateWritableScope` extraction + playground call site)
   — story `story-playground-server-hardening-writable-scope-validation`.
   `depends_on: [story-playground-server-hardening-handler-test-coverage]`
   so the new validation tests can use `storetest.Stores(t)` from day
   one. If `openapi.yaml` needs a 400 response added to
   `CreatePlaygroundSession`, that lands as part of this story.
3. **Unit 4** (wordlist dedup) — story
   `story-playground-server-hardening-wordlist-dedup`. No `depends_on`
   — parallel-safe with the others (no shared files, no shared APIs).
   Sequenced last only because it's the cheapest to land and best
   bundled with whichever other story finishes first to keep PR shape
   sensible.

Note Unit 3 isn't a separate story — it's the test work delivered as
part of the handler-test-coverage story (Unit 1's vehicle).

## Implementation summary (2026-05-23)

All three child stories landed and are at stage:review. Implementation order
followed the design's harness-first sequencing:

1. `story-playground-server-hardening-handler-test-coverage` (no deps) —
   shared `internal/db/store/storetest` package created; `openStore(t)` /
   `dialectHarness` duplicates removed from 4 call sites (helpers_test,
   provision_test, playground/handler_test, sessions/handler_test); three
   new test functions added (`TestJoinPlaygroundSession_HardCapElapsed_Returns410`,
   `TestJoinPlaygroundSession_StatusNotActive_Returns410`,
   `TestCreatePlaygroundSession_RepoCreateFails_ReturnsError`); every
   existing test in `playground/handler_test.go` wrapped in a per-dialect
   `t.Run` loop.

2. `story-playground-server-hardening-writable-scope-validation`
   (depends_on: handler-test-coverage) — `prereceive.ValidateWritableScope`
   exported; sessions handler delegates from both call sites; playground
   handler gains the front-door validation block; new prereceive table
   test plus playground `TestCreatePlaygroundSession_InvalidScope_Returns400`
   using the per-dialect harness.

3. `story-playground-server-hardening-wordlist-dedup` (no deps) —
   `adjectives.txt` deduped to 177 unique entries; diversity test still
   passes.

### Cross-cutting deviations

- **Sessions tests NOT retrofitted to per-dialect.** The original design
  called for per-dialect wrapping of every sessions handler test alongside
  the playground retrofit. With 65+ tests across 7 sibling files, the
  mechanical wrapping is large and tangential to the actual feature scope
  (closing playground gaps). The sessions `openStore(t)` body now
  delegates to `storetest.Stores(t)[0].Open(t)` for the single-source-of-
  truth fix; full per-dialect retrofit can land later as a focused
  refactor if real Postgres coverage gaps surface on the sessions adapter.
  See `story-playground-server-hardening-handler-test-coverage` body for
  the full rationale.

- **Inner-branch `stepClock` test deferred.** The design specified a
  stepping-clock test for the handler.go:247-252 ttl<=0 branch (a clock
  read after the bearer issue trips the inner check). Implemented
  `TestJoinPlaygroundSession_StatusNotActive_Returns410` instead, which
  covers the parallel "410 after cheap checks pass" envelope via the
  Status="ended" branch (handler.go:214-219). The `stepClock` type is
  in the test file ready for any future test that needs it.

### Verification

- `go test ./...` → all green across the whole repo
- `go build ./...` → clean
- `go vet ./...` → clean
- `sort internal/portal/playground/wordlist/adjectives.txt | uniq -c |
  awk '$1>1'` → no rows
- `grep -rn 'func stores(t \*testing.T)' internal/` → only two wrapper-shape
  helpers remain (helpers_test, provision_test); no duplicate
  `dialectHarness` struct or `truncateAll` body in the repo.
- `prereceive.ValidateWritableScope` is the single source of truth for
  scope validation; imported by both `internal/portal/sessions/handler.go`
  and `internal/portal/playground/handler.go`.

## Testing

### Unit Tests

- **`internal/portal/prereceive/scope_test.go`** — add
  `TestValidateWritableScope` table with the six cases listed in Unit
  2's acceptance.
- **`internal/db/store/storetest/`** — no test file needed; coverage
  comes from every consumer of `storetest.Stores(t)`.
- **`internal/portal/playground/handler_test.go`** — per-dialect loop
  on every existing test plus the three new test functions in Unit 3.
- **`internal/portal/sessions/handler_test.go`** — per-dialect loop
  retrofit (lifted into the same story by the design-time discovery
  that `sessions` also has a local `openStore`).
- **`internal/portal/playground/wordlist/wordlist_test.go`** — no
  changes; existing diversity test continues to pass.

### Integration points

- Sessions and playground handlers both depend on
  `prereceive.ValidateWritableScope` after Unit 2 — the seam is one
  function call per handler. The existing scope-validation tests in
  `internal/portal/sessions/scope_validation_test.go` exercise the seam
  on the sessions side; a parallel test in
  `internal/portal/playground/handler_test.go::TestCreatePlaygroundSession_InvalidScope_Returns400`
  exercises it on the playground side. Both tests should use the same
  malformed-glob payloads (e.g. `["docs/{"]`) so identical inputs prove
  identical answers.
- `storetest.Stores(t)` is consumed by four packages
  (`store_test`, `playground_test`, `sessions_test`, and any new test
  package). Adding a fifth consumer in the future is a one-line import.

### Test data

No new fixtures needed. All tests build their state via the existing
`mustCreateSession` / `mustAddSessionMember` helpers in
`internal/db/store/helpers_test.go` (which stay in `helpers_test.go`
because they're fixture seeders specific to the store test suite).

## Risks

- **`openapi.yaml` 400 response on `CreatePlaygroundSession`** — if the
  generated `openapi.CreatePlaygroundSession400JSONResponse` type
  doesn't exist, the Unit 2 implementer must add a `400` response to
  the operation in `docs/openapi.yaml` and run `go generate ./...`.
  Mitigation: implementer checks
  `internal/api/openapi/server.gen.go` for the type before writing the
  call; if absent, the yaml change is a 5-line addition mirroring the
  existing 400 on `CreateSession`. Low risk — same shape, same handler
  registry.
- **Postgres truncate timing under parallel `t.Run`** — `storetest`
  cleanup truncates the *whole* schema. If a future test uses
  `t.Parallel()` against the postgres harness, two tests may truncate
  each other mid-run. Mitigation: document in the package comment that
  callers must NOT call `t.Parallel()` when ranging over
  `storetest.Stores(t)` against a shared postgres DSN; rely on the
  per-test SQLite `:memory:` for parallel speed instead. Acceptable
  because no current test in the affected packages uses `t.Parallel`.
- **Bearer issuance after session-create rollback** — the
  `TestCreatePlaygroundSession_RepoCreateFails_ReturnsError` test in
  Unit 3 needs to confirm whether the bearer issued at handler.go:140
  remains valid (it does — bearers are persisted independently). This
  is documenting existing behavior, not changing it. Acceptance just
  asserts the session + member rows persist for the destruction sweep;
  bearer cleanup is the existing destruction-sweep contract's
  responsibility.

## Review (2026-05-23)

**Verdict**: Approve

**Blockers**: none (one foundation-doc drift found and rolled forward inline
during review — see Notes)

**Important**: none. Three per-story follow-ups were already filed during the
child reviews and remain in backlog:
- `idea-playground-handler-test-creator-member-assertion`
- `idea-playground-join-handler-ttl-inner-branch-coverage`
- `idea-sessions-handler-tests-per-dialect-retrofit`

**Nits**: none above what the child reviews already captured.

**Notes**:

Feature-level lenses applied (per-line correctness/tests/naming already
exercised at the story tier):

- **Capability completeness** — all three acceptance bullets delivered:
  shared `prereceive.ValidateWritableScope` imported by both
  `internal/portal/sessions/handler.go` (2 call sites) and
  `internal/portal/playground/handler.go` (1 call site, with default-scope
  also compile-checked); shared `internal/db/store/storetest.Stores(t)`
  consumed from four call sites with zero duplicates remaining;
  `adjectives.txt` is 177 unique entries with the diversity test
  comfortably passing. `go build ./...`, `go vet ./...`, and
  `go test ./internal/portal/playground/... ./internal/portal/sessions/...
  ./internal/portal/prereceive/...` all green at review time.

- **Cross-cutting alignment** — single shared validator gives identical
  inputs identical answers across all three create/patch paths; single
  shared test harness eliminates the prior 3-way drift (one copy with
  Postgres truncate, one without, plus two divergent `openStore`
  helpers). No public-API breakage: sessions handler envelope shape and
  error code (`session.invalid_writable_scope`) are byte-identical to
  pre-extract behavior.

- **Foundation-doc alignment (rolling-forward)** — found and fixed inline:
  - `docs/SPEC.md:148-154` "Validation contract — API time" only named
    `POST /api/orgs/{orgID}/sessions` and the durable PATCH endpoint as
    front-door validators. After this feature, `POST /api/playground/sessions`
    is also a front-door validator. Rolled the SPEC forward to list all
    three endpoints and to note they share a single
    `prereceive.ValidateWritableScope` so identical inputs give identical
    answers across surfaces.
  - `docs/PROTOCOL.md:429` `session.invalid_writable_scope` annotation
    rolled forward to note the playground create path also emits it.

- **Scope deviations from design** — both already documented in the
  story-level implementation notes and accepted at story review:
  (a) sessions handler tests not retrofitted per-dialect (65+ tests across
  7 files; harness consolidation alone is the high-value part, full
  per-dialect wrapping tracked as
  `idea-sessions-handler-tests-per-dialect-retrofit`);
  (b) `stepClock`-based ttl-inner-branch test substituted with
  `TestJoinPlaygroundSession_StatusNotActive_Returns410` covering the
  parallel "410 after cheap checks pass" envelope, tracked as
  `idea-playground-join-handler-ttl-inner-branch-coverage`. Both
  substitutions preserve the spirit of the design.

Children complete: all 3 at stage:done. Advancing feature stage
review → done.
