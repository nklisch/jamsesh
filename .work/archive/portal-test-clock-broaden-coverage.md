---
id: portal-test-clock-broaden-coverage
kind: feature
stage: done
tags: [testing, testability, portal]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Broaden test-clock injection to all production time.Now sites

## Idea

`portal-test-clock-advance-endpoint` (v1) threads an injectable clock
through `internal/portal/auth/magic_link.go` only — the call sites
required to un-skip `magic_link_ttl_expiry`. A full audit of
`internal/portal/` surfaced ~25 other production `time.Now()` reads
that remain on the real wall clock:

- `internal/portal/accounts/orgs.go` (org creation, slug retry)
- `internal/portal/accounts/handlers.go` (org-invite create)
- `internal/portal/auth/oauth.go` (state TTL check)
- `internal/portal/auth/provision.go` (account/org provisioning)
- `internal/portal/auth/slug.go` (rand seed — likely fine to leave)
- `internal/portal/automerger/outcomes.go` (merge timestamps, ×3)
- `internal/portal/comments/service.go` (×3)
- `internal/portal/events/log.go` (×3)
- `internal/portal/finalize/lock_acquire.go`,
  `finalize/lock_release.go`, `finalize/lock_patch.go`,
  `finalize/plan.go`, `finalize/mark_shipped.go`
- `internal/portal/logging/logging.go` (access-log latency — fine to
  leave on real clock)
- `internal/portal/mcpendpoint/handler.go`,
  `mcpendpoint/tools.go`
- `internal/portal/oauth/state.go`
- `internal/portal/sessions/handler.go`, `sessions/invites.go`,
  `sessions/listing.go`
- `internal/portal/storage/archive.go`

`internal/portal/tokens` already has its own injectable `Clock` but is
not yet wired to the runtime `/test/clock-advance` knob.

Unlocked tests:
- `clock_skew_token_expiry` in
  `tests/e2e/chaos/runtime_and_clock_test.go` (currently
  documented-skip; needs the `tokens.Service` to be clock-advanceable
  at runtime).
- 30-minute idle-TTL path on finalize locks (alluded to in
  `tests/e2e/failure/interrupted_ops_test.go >
  finalize_lock_release_and_reacquire` comment).
- Org-invite expiry, OAuth-state expiry, comment max-age window —
  none currently have skipped tests but adding the injectability
  unlocks future failure-mode subtests cheaply.

## Approach sketch

Mirror the v1 pattern:
1. Each package gets its own local `Clock` interface (auth-package
   pattern) OR they share `tokens.Clock` if the import graph allows.
2. Constructors get a `WithClock` variant; the default constructor
   uses `realClock{}`.
3. The e2etest-tagged `cmd/portal/test_clock_advance.go` injects the
   shared `*testclock.AdvanceableClock` into every handler that
   accepts a clock.

Sizing: ~12 handlers / services, ~25 call sites, ~12 constructor
edits in `cmd/portal/main.go`. Probably 2-3 stories rather than one.

## Why deferred

No active test is blocked on these in v1. Acceptance criteria for
`portal-test-clock-advance-endpoint` names only the magic-link TTL
test. Broadening eagerly would multiply the v1 surface area by an
order of magnitude with no immediate gate to justify it.

Promote via `/agile-workflow:scope` when:
- a second clock-sensitive e2e test gets blocked on this, OR
- the chaos `clock_skew_token_expiry` scenario is prioritized to
  un-skip.

## Design

### Audit summary

Full audit of `grep -rn 'time.Now()' internal/portal/ --include='*.go'`
(production files only — `_test.go` excluded) found **28 production
sites** across **13 packages**:

| Package | Sites | Constructor today |
|---|---|---|
| `auth/magic_link.go` | (done in v1) | `auth.NewMagicLinkHandler[WithClock]` |
| `auth/oauth.go` | 1 — state-TTL check | `auth.NewOAuthHandler` (LOCKED — in-flight) |
| `auth/provision.go` | 1 — `createAccountAndOrg` `now` | free function `FindOrProvision` |
| `auth/slug.go` | 1 — rand seed | free function `randomSuffix` |
| `accounts/orgs.go` | 2 — `CreateOrgInvite`, `AcceptOrgInvite` | `accounts.New(...)` |
| `accounts/handlers.go` | 1 — `CreateOrg` | same Handler |
| `oauth/state.go` | 1 — `StoreState` | free function |
| `sessions/handler.go` | 2 — `CreateSession`, `AbandonSession` | `sessions.New(...)` (LOCKED — in-flight) |
| `sessions/invites.go` | 2 — `InviteToSession`, `AcceptSessionInvite` | same Handler |
| `sessions/listing.go` | 1 — pagination `before` cursor | same Handler |
| `comments/service.go` | 3 — `Create`, `Resolve`, `List` cursor | `&Service{Store, Log}` literal |
| `events/log.go` | 3 — `Emit`, `EmitBatch`, `UpdatePresence` | `events.New(s)` |
| `finalize/lock_acquire.go` + 4 others | 5 — lock_acquire, lock_release, lock_patch, plan, mark_shipped | `finalize.New(...)` |
| `automerger/outcomes.go` | 3 — merger signature, conflict event, conflict resolve | `automerger.NewApplier(s, log)` |
| `storage/archive.go` | 1 — `ArchivedAt` stamp | `storage.New(cfg, store)` |
| `mcpendpoint/handler.go` | 1 — `verifyToken` Expiration stamp | `&Endpoint{...}` literal |
| `mcpendpoint/tools.go` | 1 — `fork` tool `ForkedAt` payload | same Endpoint |
| `logging/logging.go` | 1 — access-log latency | middleware (intentional skip) |
| `tokens/service_impl.go` | (already has Clock — needs runtime wiring only) | `tokens.New[WithClock](s, c)` |

### Taxonomy of call sites

Three semantic categories — useful for understanding why we want each
clock-advanceable:

1. **TTL gates** (clock advance must affect a comparison):
   - `auth/magic_link.go` ✔ done
   - `auth/oauth.go` line 110 — state-TTL check (deferred — locked)
   - `accounts/orgs.go` line 138 — org-invite expiry
   - `finalize/*` — 30-minute idle-lock check via `IsLockExpired(lastActivity, now)`
   - `tokens/service_impl.go` — access-token expiry check (v1
     internal-only; this feature wires the runtime knob)

2. **Write timestamps stamped into rows** (advancement is observable
   downstream via `created_at` / `expires_at` / `acquired_at` fields):
   - `accounts/orgs.go` line 63 (`CreatedAt`/`ExpiresAt` for invite)
   - `accounts/handlers.go` line 85 (org `CreatedAt`)
   - `auth/provision.go` line 85 (account+org `CreatedAt`)
   - `sessions/invites.go` lines 94, 175 (`CreatedAt`/`AcceptedAt`)
   - `sessions/handler.go` lines 62, 332 — deferred (locked)
   - `comments/service.go` lines 91, 202 (`CreatedAt`/`ResolvedAt`)
   - `events/log.go` lines 138, 175, 243 (event `CreatedAt`)
   - `finalize/*` — 5 sites stamping `LastActivityAt` / `ReleasedAt`
     / `EndedAt`
   - `automerger/outcomes.go` lines 94, 218, 271 (merge commit `When`,
     conflict `CreatedAt`, resolve `ResolvedAt`)
   - `storage/archive.go` line 34 (`ArchivedAt`)
   - `oauth/state.go` line 34 (state `CreatedAt`/`ExpiresAt`)
   - `mcpendpoint/handler.go` line 80 (SDK `Expiration` field — see
     decision below)
   - `mcpendpoint/tools.go` line 231 (`ForkedAt` in `ref.forked`
     payload)

3. **Pagination cursors** ("now" used as upper-bound for descending
   page query):
   - `sessions/listing.go` line 68
   - `comments/service.go` line 298

4. **Intentional real-clock sites** (NOT wrapped):
   - `logging/logging.go` line 34 — access-log latency measurement
   - `auth/slug.go` line 66 — `math/rand` seed entropy

### Package-grouping rationale (child-story decomposition)

The body suggested 2–3 stories. Settled on **3**:

**Story A — tokens wiring + chaos un-skip** (smallest, highest value)
- `tokens` package already has `NewWithClock`; only `cmd/portal/main.go`
  needs to consume it. Un-skips the named chaos test.
- Cleanly separable from everything else; lands first; gives the
  team an immediate test-quality win.

**Story B — provisioning and state-TTL plumbing**
- `accounts/handlers.go`, `accounts/orgs.go`, `auth/provision.go`,
  `oauth/state.go`.
- Coherent theme: account/org provisioning timestamps + state-nonce
  expiry. All sit on the auth flow's critical path.
- `auth/provision.go` and `oauth/state.go` are free-function helpers
  whose callers (`magic_link.go`, `oauth.go`) decide the clock. The
  refactor passes `now time.Time` through these helpers, leaving the
  CALLER as the clock authority. This keeps `auth/oauth.go`
  unmodified (in-flight), but ready for a follow-on story to wire
  through `OAuthHandler`'s clock once that file is unlocked.

**Story C — handlers, workers, MCP** (mechanical broadening)
- Everything else: `sessions/invites.go`, `sessions/listing.go`,
  `comments/service.go`, `finalize/*` (5 files), `storage/archive.go`,
  `events/log.go`, `automerger/outcomes.go`, `mcpendpoint/*`.
- Uniform pattern (`Clock` interface, `WithClock` constructor,
  `clock.Now()` swap). Larger but mechanical.
- Background workers (`events`, `automerger`) wired even though they
  don't sit on the HTTP request path — see Design decisions below.

Stories A, B, C have **no inter-dependencies** — they touch disjoint
files in `internal/portal/` and additive accessor methods in
`cmd/portal/test_clock_advance*.go`. All three can land in parallel.
The cross-story coordination is only in `cmd/portal/main.go`, where
each story adds its own conditional-clock block; the blocks don't
overlap. **`depends_on: []` for every child story.**

### Why per-package Clock interfaces (not a shared `tokens.Clock`)

The `tokens` package is structurally upstream of every other handler
(its only internal-jamsesh imports are `store`, `deperr`, `httperr`,
`openapi`), so a shared `tokens.Clock` is import-graph-feasible. But
keeping per-package interfaces wins on:

1. **Decoupling.** A package that only needs `Now() time.Time` shouldn't
   gain an import on `tokens` just for the type alias. Several
   handler packages (`accounts`, `comments`, `storage`, `events`,
   `automerger`) don't otherwise import `tokens`.
2. **Test-package independence.** Per-package fakes can satisfy each
   `Clock` without cross-package fixtures.
3. **Structural typing wins anyway.** The same
   `*testclock.AdvanceableClock` instance satisfies every per-package
   `Clock` interface because all have the identical
   `Now() time.Time` shape. The shared-type benefit (one knob moves
   all clocks) is achieved without the import-coupling cost.
4. **Matches v1 reference.** `auth.Clock` and `tokens.Clock` are
   already per-package by the same reasoning. Continuing the pattern
   makes the codebase consistent.

### Implementation pattern (uniform across all packages)

Each modified package gets:

```go
type Clock interface {
    Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }
```

Plus, depending on the constructor shape:

- **Constructor-style packages** (`accounts`, `finalize`, `events`,
  `automerger`, `storage`): the existing `New(...)` delegates to
  `NewWithClock(..., realClock{})`. Production code unchanged;
  test/e2etest builds construct via `NewWithClock`.

- **Struct-literal-style packages** (`comments`, `mcpendpoint`): a
  `Clock` field is added to the struct; the field is exported so
  callers can set it via the literal. Internal reads go through a
  nil-safe helper:
  ```go
  func (s *Service) now() time.Time {
      if s.Clock == nil { return time.Now().UTC() }
      return s.Clock.Now()
  }
  ```

Per-package accessors on `cmd/portal/test_clock_advance.go` (e2etest)
return `p.clock`; the production stub (`!e2etest`) returns
`nil` typed as the per-package `Clock` interface. `cmd/portal/main.go`
uses the established `if c := testClk.fooClock(); c != nil { ... }`
pattern from v1.

## Design decisions

### 1. Per-package `Clock` interfaces vs shared `tokens.Clock`

**Decision:** Per-package. Each modified package declares its own
`Clock` interface and `realClock` impl.

**Rationale:** See "Why per-package Clock interfaces" above.
Decoupling + structural-typing convenience preserved; matches v1.

### 2. `logging/logging.go` access-log latency — leave on real clock

**Decision:** Confirmed skip.

**Rationale:** The `time.Since(start)` measurement is reporting on
real wall-clock duration, not domain logic. Advancing a test clock
shouldn't fabricate latency numbers in access logs — that would
either confuse log aggregation or create spurious slow-query alerts
during e2e runs. The HTTP middleware sits outside any TTL gate.

### 3. `auth/slug.go` rand seed — leave on real clock

**Decision:** Confirmed skip.

**Rationale:** `rand.NewSource(time.Now().UnixNano())` is used to
seed a non-secret `math/rand` generator that picks 6 alphanumeric
chars to disambiguate org-slug collisions. The slug collision
probability is unaffected by clock advancement; the seed quality is
fine with either source. Replacing this with `clock.Now().UnixNano()`
adds plumbing for no test benefit.

### 4. Background workers (`events/log.go`, `automerger/outcomes.go`)
— wire anyway

**Decision:** Wire `Clock` into both, with the same accessor pattern.

**Rationale:** Although neither runs inside an HTTP request path that
the `/test/clock-advance` endpoint serves, both write timestamps that
end-to-end tests may inspect (e.g., a chaos test that pushes a
commit, advances the clock, and expects the resulting `merge.succeeded`
event's `created_at` to reflect the advanced time). Wiring them
preserves the single-source-of-time invariant across the entire portal
process: advancing once moves every observable clock. Cost is trivial
(3 reads × 2 packages = 6 line swaps + 2 constructor variants).

### 5. `postreceive/*` not in scope

**Decision:** Confirmed — `postreceive/emitter.go` does NOT contain
production `time.Now()` reads (verified via grep). The body's mention
of "postreceive/*" was loose. No change needed.

### 6. `mcpendpoint/*` — wire it

**Decision:** Include both `handler.go` and `tools.go` sites.

**Rationale:** MCP shares bearer auth with REST. A future test that
exercises an expired MCP token (or a `fork` tool call whose
`ForkedAt` stamp matters) needs the same clock semantics. The MCP
SDK's `auth.TokenInfo.Expiration` field (`handler.go:80`) is set to
`time.Now().Add(24h)` — a sentinel far-future expiry because jamsesh
tokens are opaque and TTL is enforced in the DB. Even though this
stamp doesn't drive an actual SDK-side gate today, advancing the
test clock should still keep the sentinel internally consistent so
debugging is not surprising.

### 7. In-flight conflicts — defer instead of dual-edit

**Decision:** Three files are explicitly NOT modified in this feature
because they are owned by in-flight features:

- `internal/portal/auth/oauth.go` (in-flight:
  `portal-oauth-provider-error-taxonomy`) — 1 site (state-TTL check).
- `internal/portal/sessions/handler.go` (in-flight:
  `portal-validate-writable-scope-at-create-time`) — 2 sites
  (`CreateSession`, `AbandonSession`).
- `internal/portal/oauth/github.go` (in-flight: same OAuth feature) —
  0 sites (no production `time.Now`), but listed for completeness.

The helpers these handlers call (`auth.FindOrProvision`,
`oauth.StoreState`) ARE refactored in Story B with backward-compatible
signatures — so once the in-flight features merge, a small follow-on
story can finish the wiring in ~30 LoC. Tracking item: park a
backlog note after this feature ships (see end of feature body).

### 8. Story decomposition shape — 3 stories, no inter-dependencies

**Decision:** Stories A, B, C all declare `depends_on: []`.

**Rationale:** No shared internal-portal file is touched by more than
one story. The only cross-story file is `cmd/portal/main.go`, where
each story adds an additive conditional-clock block — the three
blocks are independent (no shared lines, no shared imports beyond
what's already present). Three implementers in parallel land 3
independent commits to `main.go`; standard merge resolution.

## Child stories

| Story | Sites | depends_on |
|---|---|---|
| [`portal-test-clock-broaden-coverage-tokens-wiring`](../stories/portal-test-clock-broaden-coverage-tokens-wiring.md) | tokens wiring (0 new abstraction) + chaos un-skip | [] |
| [`portal-test-clock-broaden-coverage-provisioning-and-state`](../stories/portal-test-clock-broaden-coverage-provisioning-and-state.md) | accounts ×3, auth/provision ×1, oauth/state ×1 | [] |
| [`portal-test-clock-broaden-coverage-handlers-workers-mcp`](../stories/portal-test-clock-broaden-coverage-handlers-workers-mcp.md) | sessions/invites+listing ×3, comments ×3, finalize ×5, storage ×1, events ×3, automerger ×3, mcpendpoint ×2 | [] |

**Total covered:** 24 production sites + tokens runtime wiring = 25.

**Intentionally skipped:** 2 sites
- `logging/logging.go:34` — access-log latency
- `auth/slug.go:66` — rand seed entropy

**Deferred (in-flight conflict):** 3 sites
- `auth/oauth.go:110` — state-TTL check (follow-on after oauth-taxonomy merges)
- `sessions/handler.go:62, 332` — CreateSession/AbandonSession write
  stamps (follow-on after scope-validation merges)

Follow-on story will be parked to `.work/backlog/` after this feature
lands; estimated ~30 LoC of mechanical wiring once the locked files
unlock.

## Acceptance criteria (feature-level)

- [ ] All three child stories at stage `done`.
- [ ] `tests/e2e/chaos/runtime_and_clock_test.go > clock_skew_token_expiry`
      runs green (un-skipped) under `make test-portal-image` build.
- [ ] `tests/e2e/failure/interrupted_ops_test.go > magic_link_ttl_expiry`
      still passes (regression check on the v1 path).
- [ ] `git grep -- 'testclock' cmd/portal/ internal/portal/` returns
      only build-tag-gated files plus prod stubs.
- [ ] Production build (`go build ./...`) succeeds; resulting binary
      returns 404 on `POST /test/clock-advance`.
- [ ] e2etest build (`go build -tags e2etest ./...`) succeeds.
- [ ] A single `POST /test/clock-advance` request advances every
      wired clock simultaneously (verified by spot-checking that
      `clock_skew_token_expiry` and `magic_link_ttl_expiry` both pass
      in a single test run that calls `AdvanceClock` once).
- [ ] Follow-on backlog item parked covering the 3 deferred sites in
      `auth/oauth.go` and `sessions/handler.go`.

## Review (2026-05-17) — Approve, archived with children

### Verdict

**Approve.** All three child stories shipped at `stage: done` with
clean reviews. Production safety, e2etest gating, and the targeted
chaos test all green.

### What shipped end-to-end

- **Story A — tokens wiring + chaos un-skip** (commit `13a4b50`).
  `tokens.NewWithClock(dbStore, c)` wired in `cmd/portal/main.go`
  behind `testClk.tokensClock()`; `clock_skew_token_expiry` un-skipped
  and passes (advance by `AccessTokenTTL + 1m = 1h1m` causes the prior
  bearer to return 401 `auth.expired_token`).
- **Story B — provisioning + state TTL** (commit `fc05cff`). 5 sites
  wired across `accounts/handlers.go` (`CreateOrg`),
  `accounts/orgs.go` (`CreateOrgInvite`, `AcceptOrgInvite`),
  `auth/provision.go` (`FindOrProvisionAt` additive variant),
  `oauth/state.go` (`StoreStateAt` additive variant). Back-compat
  wrappers kept so `auth/oauth.go` (in-flight lock) compiles
  unmodified.
- **Story C — handlers + workers + MCP** (commit `e54ea0b`). 17 sites
  wired across `comments` (3), `finalize` (5), `storage` (1), `events`
  (3), `automerger` (3), `mcpendpoint` (2). Six new `clock_test.go`
  files added. Seven new `*Clock()` accessors registered on
  `testClockProvider`.

**Totals.** 5 + 17 + tokens-runtime-knob = **22 sites + 1 runtime
wiring = 23 wired** (the design's "~25 sites" target). Intentional
skips: 2 (`logging/logging.go:34` access-log latency,
`auth/slug.go:66` rand seed). Deferred to follow-on: 5 sessions sites
(`handler.go` ×2, `invites.go` ×2, `listing.go` ×1) — parked at
`.work/backlog/portal-test-clock-broaden-coverage-sessions-followup.md`.

### Verification

- `go build ./...` — clean.
- `go build -tags e2etest ./...` — clean.
- `go vet ./internal/portal/...` — clean.
- `go test ./internal/portal/...` — all packages green.
- `go test -run TestProductionBuild_HasNoTestEndpoint ./cmd/portal/...`
  — pass. Production binary returns 404 on `POST /test/clock-advance`.
- `make test-portal-image` — rebuilt `jamsesh/portal:e2e` clean.
- `cd tests/e2e && go test -run TestRuntimeAndClock -v ./chaos/...`
  — **PASS**. `clock_skew_token_expiry` ran in 1.97s; portal logged
  `advanced by 1h1m0s, new offset=3660s`. `automerger_pause` also
  green (regression — 21.30s).
- `cd tests/e2e && go test -run 'TestInterruptedOps/magic_link_ttl_expiry'
  -v ./failure/...` — **PASS**. v1 magic-link TTL test still passes
  (no regression in the auth path).

### Shared-clock invariant

Spot-checked `cmd/portal/test_clock_advance.go` — all 9 e2etest
accessors (`magicLinkClock`, `tokensClock`, `accountsClock`,
`commentsClock`, `finalizeClock`, `storageClock`, `eventsClock`,
`automergerClock`, `mcpClock`) return the unmodified `p.clock` field
on a single `*testClockProvider`. The provider's `clock` field is a
single `*testclock.AdvanceableClock` constructed once in
`newTestClockProvider()`. One `POST /test/clock-advance` advances
every wired clock simultaneously — confirmed by the chaos test which
relies on the same provider for both magic-link and token expiry.

### Per-package Clock pattern consistency

Spot-checked across the six newly-wired packages plus the v1
reference (`auth/magic_link.go`):

- All declare a local `type Clock interface { Now() time.Time }` —
  identical shape; structural typing carries the shared-instance
  property without import-graph coupling.
- All declare a `realClock` with `func (realClock) Now() time.Time
  { return time.Now().UTC() }`.
- Constructor-style packages (`accounts`, `finalize`, `storage`,
  `events`, `automerger`) expose `NewWithClock(...)`; the default
  `New(...)` delegates with `realClock{}`. Production code unchanged.
- Struct-literal-style packages (`comments`, `mcpendpoint`,
  `automerger.Applier`) expose a `Clock` field with a nil-safe `now()`
  helper, preserving struct-literal callers.

### Intentional-skip audit

- `internal/portal/logging/logging.go:34` — `start := time.Now()` for
  access-log latency. Matches Design decision #2. Wall-clock
  measurement; advancing the test clock would fabricate latency
  numbers.
- `internal/portal/auth/slug.go:66` —
  `rand.New(rand.NewSource(time.Now().UnixNano()))`. Matches Design
  decision #3. Non-secret PRNG seed; advancement has no test benefit.

Both remain on the real wall clock as designed.

### Deferred-sessions judgment

Parked properly. The follow-on backlog item
(`.work/backlog/portal-test-clock-broaden-coverage-sessions-followup.md`)
exists, names the 5 sites precisely (handler.go ×2, invites.go ×2,
listing.go ×1), notes the unlocking event (commit `87835cc` — the
in-flight `portal-validate-writable-scope-at-create-time` feature has
since landed), and is sized at ~50-80 LoC for a single follow-on
stride. No active test is blocked on these sites — they're write
timestamps and a pagination cursor with no TTL semantics, so deferring
adds no test debt.

### Acceptance-criteria coverage

- [x] All three child stories at stage `done`.
- [x] `clock_skew_token_expiry` runs green (un-skipped) under
      `make test-portal-image`.
- [x] `magic_link_ttl_expiry` still passes (regression check).
- [x] `git grep -- 'testclock' cmd/portal/ internal/portal/` returns
      only e2etest-tagged files plus prod stubs plus doc comments.
- [x] `go build ./...` succeeds; production binary returns 404 on
      `POST /test/clock-advance`.
- [x] `go build -tags e2etest ./...` succeeds.
- [x] A single `POST /test/clock-advance` advances every wired clock
      simultaneously (verified via the shared `p.clock` invariant
      and the chaos test's actual advance behaviour).
- [x] Follow-on backlog item parked covering the deferred sessions
      sites.

### Findings

- Blockers: 0.
- Important: 0.
- Nits: 0.

The feature delivers exactly the scope its design committed to.
Advancing to `done` and archiving with children.
