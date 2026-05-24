---
id: feature-e2e-playground-coverage-failure
kind: feature
stage: implementing
tags: [testing, e2e-test, playground, portal]
parent: epic-e2e-playground-coverage
depends_on: [feature-e2e-playground-coverage-golden]
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# Failure-mode e2e tests for the playground subsystem

## Brief

Failure-mode end-to-end coverage for the v0.4.0 playground subsystem.
Tests assert that the documented failure responses (429 rate-limit, 413/
pre-receive content-cap rejection, 401 bearer expiry, exit-1 reserved-slug
boot conflict) actually fire against the real portal binary in a
Testcontainers stack — not just against the unit-suite's stubbed clock
and stubbed storage.

Depends on `feature-e2e-playground-coverage-golden` because failure
tests reuse the patterns golden establishes: fixture composition,
bearer-header injection through the `binary` fixture, the
`dockerExec`-based assertion shape, and the `/test/clock-advance`
helper for time-bounded failure modes (bearer expiry).

## Child stories

This feature has 4 child stories, all carried over from the
`e2e-test-design --audit` run:

1. `e2e-audit-playground-rate-limit-abuse-cap` (High) — 4th
   `POST /api/playground/sessions` from the same IP within an hour gets
   a real 429, with the `Retry-After` header populated
2. `e2e-audit-playground-content-cap-pre-receive-enforcement` (High) —
   a push that pushes total repo size past 50 MiB is rejected at the
   real `pre-receive` hook (not just at the unit-level
   `prereceive.ValidateContentCap` function)
3. `e2e-audit-playground-bearer-expiry-hard-cap` (High) — uses
   `/test/clock-advance` to skip past hard-cap; subsequent bearer use
   returns the documented 401 with the right error envelope
4. `e2e-audit-playground-reserved-org-slug-boot-conflict` (Medium) —
   boots the portal binary with `--playground-enable` against a DB
   pre-seeded with a non-protected `playground` slug; asserts the
   binary exits 1 with the documented error message (goes in
   `tests/e2e/failure/config_and_deps_test.go` alongside existing
   boot-failure tests)

## Design status

Same as golden — the audit supplied sketches; e2e-test-design's job is
to lock the mock-boundary plan, surface ambiguities (e.g. how does the
rate-limit test reset between subtests without polluting shared portal
state?), and pre-mortem the suite.

## Mock-boundary plan

Identical to golden's plan (inherited). Every external dep is covered by
existing fixtures at `tests/e2e/fixtures/`: `postgres`, `portal`,
`gitclient`, `wsclient`, `binary`. No new custom containers. Two
additional patterns the failure tests exercise:

- **`portal.Logs(ctx)`** for the boot-failure subtest of
  `reserved-org-slug-boot-conflict` — to assert on container log output
  when the portal exits non-zero. Pattern source: existing
  `tests/e2e/failure/config_and_deps_test.go > TestConfigAndDeps`.
- **`postgres.Exec` (or a direct SQL connection via `pg.DSN`)** for
  pre-seeding the `orgs` row in the same test. Pattern verified by the
  postgres fixture's exposed DSN field.

One small known gap: `portal.Start` is fatal-on-failure (calls
`t.Fatalf` inside the goroutine if healthz never lands). The
reserved-slug test needs a TryStart variant OR a direct
testcontainers bypass to detect intentional boot failures. The
simpler path: extend the postgres fixture's pre-seed pattern so the
slug-conflict scenario can be set up BEFORE `portal.Start` runs, then
either (a) the portal handles the conflict gracefully (advancing to
"take ownership" outcome, no fatal) or (b) accept the t.Fatal and
read container logs in t.Cleanup. The reserved-slug story owner can
make that call during implementation.

## Taxonomy plan

This feature is the **failure** layer only. 4 tests covering distinct
failure modes:

- Rate-limit abuse cap (429 from per-IP/hour limiter)
- Content-cap pre-receive rejection (50 MiB cap → push fails)
- Bearer expiry at hard-cap (401 after AdvanceClock past hard-cap)
- Reserved-org slug boot conflict (portal exits 1 OR takes ownership)

## Anti-tautology guardrails

Inherited from golden — every test asserts on user-visible outcomes
(real HTTP response codes + headers, real on-disk state, real container
log output), runs against the real portal binary, states its invariant
in plain English at the top of the function. No mock invocations, no
in-process `httptest.NewServer`.

## Implementation Units

### Unit 1: rate-limit abuse cap
**File**: `tests/e2e/failure/playground_rate_limit_abuse_cap_test.go`
**Story**: `e2e-audit-playground-rate-limit-abuse-cap` (High)
**Invariant**: With `JAMSESH_PLAYGROUND_CREATE_PER_IP_HOUR=3`, the
first 3 anonymous `POST /api/playground/sessions` requests from the
same client succeed (201) and the 4th is rejected with 429 +
`Retry-After` header + `playground.rate_limited` error envelope. A
parallel test client (different source) demonstrates the limiter is
per-IP not global.

### Unit 2: content-cap pre-receive enforcement
**File**: `tests/e2e/failure/playground_content_cap_test.go`
**Story**: `e2e-audit-playground-content-cap-pre-receive-enforcement` (High)
**Invariant**: With `JAMSESH_PLAYGROUND_MAX_CONTENT_BYTES=1<<20` (1 MiB
for test speed), a push that brings the session's accumulated repo
size past the cap is rejected at the real `git-receive-pack`
subprocess pipeline, and the on-disk repo size after the rejection is
still at or below the cap (no partial-write commit).

### Unit 3: bearer expiry at hard-cap
**File**: `tests/e2e/failure/playground_bearer_expiry_hard_cap_test.go`
**Story**: `e2e-audit-playground-bearer-expiry-hard-cap` (High)
**Invariant**: Anonymous bearer issued by `POST /sessions` is rejected
with 401 after `p.AdvanceClock` past the hard-cap. Cross-session
isolation also tested: bearer B1 hitting session S2 must 401.

### Unit 4: reserved-org slug boot conflict
**File**: `tests/e2e/failure/playground_reserved_slug_boot_conflict_test.go`
**Story**: `e2e-audit-playground-reserved-org-slug-boot-conflict` (Medium)
**Invariant**: Booting the portal with playground enabled against a DB
where the `playground` slug is held by an UNPROTECTED org (org_protected
= false) either exits non-zero with a clear error in container logs, OR
takes ownership (sets org_protected=true on the existing row) — both
are valid outcomes per the audit's documented contract.

## Implementation Order

1. Unit 1 (rate-limit) — establishes the failure-test pattern;
   simplest invariant (HTTP rejection codes).
2. Unit 3 (bearer-expiry) — reuses Unit 1's pattern + the
   AdvanceClock idiom already proven in golden's Unit 4. Cross-session
   isolation subtest is independent.
3. Unit 2 (content-cap) — most complex; requires generating a
   large-ish packfile and asserting on the pre-receive rejection.
   Lands third so the prior patterns are stable.
4. Unit 4 (reserved-slug boot conflict) — requires DB pre-seeding +
   either fixture extension or direct testcontainers bypass. Lands
   last; its pattern is distinct from the other 3.

## Test integrity

Inherited from golden. Park production bugs, don't hide them. Fix bad
tests in-session. Never game an assertion. Document `t.Skip` with
backlog id if a real product bug blocks completion.

## Risks (pre-mortem)

- **Rate-limit state isolation across subtests** (Unit 1). The per-IP
  limiter is process-global on the portal. Sub-tests within one
  `TestPlaygroundRateLimit_*` test share the same portal instance, so
  the limiter state from one subtest contaminates the next.
  Mitigation: each subtest spins up its own portal fixture, OR uses a
  distinct test source-IP via the proxy fixture's X-Forwarded-For.
  Pick whichever is cheaper.
- **Pack generation for content-cap test** (Unit 2). Generating a
  realistic packfile > 1 MiB requires `gitclient` to expose a
  "push N bytes" helper. If absent, the test needs an inline
  packfile-builder. Mitigation: extend gitclient if needed (small
  delta) or generate via `dd if=/dev/urandom ... && git add && git
  commit && git push`.
- **`portal.Start` fatal-on-failure** (Unit 4). See Mock-boundary
  plan's note. Resolution deferred to the implementer; document the
  decision in the story body.

## Status

Design complete. Feature advanced to `implementing`. The 4 child
stories are also advanced to `implementing` so `implement-orchestrator`
can pick them up in waves.

## Next

`/agile-workflow:implement-orchestrator feature-e2e-playground-coverage-failure`
— spawns sub-agents per the implementation order above. Units 1, 2, 3
have no story-level deps; Unit 4 implicitly depends on Unit 1 for
pattern establishment.
