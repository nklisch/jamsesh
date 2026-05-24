---
id: epic-e2e-playground-coverage
kind: epic
stage: drafting
tags: [testing, e2e-test, playground, portal, plugin]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# E2E coverage for the v0.4.0 playground subsystem

## Brief

The v0.4.0 release shipped the ephemeral anonymous "playground" subsystem
end-to-end — REST endpoints, anonymous bearer auth, reserved `playground`
org, destruction worker, per-IP rate limiting, per-session content caps,
the `jamsesh jam --playground` CLI flow, and the `/jamsesh:jam` slash
command consolidation. The entire surface ships with **zero references**
in `tests/e2e/`:

```
$ grep -rIn -E "playground|anonymous|anon\b|/api/playground|jamsesh jam" tests/e2e/
(no output — zero hits)
```

All current playground/CLI test coverage lives at unit scope in
`internal/portal/playground/*_test.go` (`handler_test.go` ≈ 1300 lines,
plus `destruction_test.go`, `worker_test.go`, `ratelimit_test.go`,
`provision_test.go`) and in `cmd/jamsesh/jamcmd/jam_test.go`. Those tests
run against `httptest.NewServer` + `stubStorage` map + `fixedClock` — no
real git, no real Postgres, no real wall clock, no real binary, no real
network. A whole shipping feature with no end-to-end verification means
every production regression — bearer issuance against a real DB,
content-cap enforcement at a real pre-receive hook, destruction worker
against a real `ticker-sweep-loop`, tombstone serving after real Postgres
TX commit, clock-injection completeness across all handler paths — will
only surface when users hit prod.

The pre-existing parked test failures
`TestJoinPlaygroundSession_Success` and
`TestJoinPlaygroundSession_WithNickname_UsesIt` (documented in
`.work/backlog/bug-playground-join-with-nickname-returns-410-on-fresh-session.md`)
are exactly the regression class an e2e suite would have caught: the
likely defect is a clock-injection mismatch between handler and test
that a fixed-clock unit suite silently swallows but real wall time
exposes immediately.

The audit that produced this epic ran via
`/agile-workflow:e2e-test-design --audit` against `tests/e2e/` and
returned 12 findings, all reproduced below as child stories under the
four taxonomy features.

## Scope

This epic adds end-to-end coverage for **only** the v0.4.0 playground +
CLI surface. It does not retroactively expand coverage of other
subsystems and does not change the existing 43 tests
(17 golden + 14 failure + 8 chaos + 4 fuzz). No new fixtures are needed —
the existing Testcontainers-Go fixtures (`portal`, `portalcluster`,
`postgres`, `mailhog`, `wiremock`, `toxiproxy`, `binary`, `ccdriver`,
`gitclient`, `wsclient`, `containerlog`) already cover every dep the
playground touches. The work is purely additive.

## Decomposition

Four child features, one per test-program taxonomy layer. The taxonomy
mirrors the existing `tests/e2e/{golden,failure,chaos,fuzz}/` directory
layout so new tests slot in naturally.

| Feature | Stories | Drives |
|---|---|---|
| `feature-e2e-playground-coverage-golden` | 5 | Happy-path journeys end-to-end (solo create + push + tombstone, two-participant join + merge, CLI `jam --playground`, abandonment → destruction sweep) plus the cross-cutting unit-vs-e2e tautology cleanup discipline |
| `feature-e2e-playground-coverage-failure` | 4 | Failure-mode coverage (rate-limit 429, content-cap rejection, bearer expiry, reserved-slug boot conflict) |
| `feature-e2e-playground-coverage-chaos` | 1 | Chaos: destruction during in-flight push |
| `feature-e2e-playground-coverage-fuzz` | 1 | Nickname input fuzzing |

Dependency ordering (cross-feature):

- **golden** has no deps — it establishes the playground e2e patterns
  (fixture composition, dockerExec assertion shape, /test/clock-advance
  usage, real-bare-repo path checks).
- **failure**, **chaos**, **fuzz** all depend on **golden** completing —
  they reuse golden's patterns rather than re-deriving them. Per the
  e2e-test-design skill: "chaos verifies graceful degradation of
  already-tested paths."

## Closing condition

This epic closes when:

1. All four child features are at `stage: done`
2. The coverage matrix has at least one passing test in every cell:
   ```
                 | Playground REST | CLI jam | Destruction | Bearer auth | Rate limit | Content cap
   Golden        | ✓               | ✓       | ✓           | ✓ (via golden) | -          | -
   Failure       | -               | -       | -           | ✓ (expiry)  | ✓          | ✓
   Chaos         | -               | -       | ✓ (during push) | -       | -          | -
   Fuzz          | -               | -       | -           | -           | -          | -  (nickname only)
   ```
   (Dashes are intentional gaps — not every cell needs coverage in this
   epic; the matrix documents what shipped, not what's exhaustive.)
3. `grep -rIn -E "playground|/api/playground|jamsesh jam" tests/e2e/`
   returns a meaningful count (currently zero)
4. The parked bug
   `bug-playground-join-with-nickname-returns-410-on-fresh-session` is
   either resolved as a side effect of e2e coverage exposing it, or
   re-parked with the e2e test that surfaces it documenting the failure
   mode honestly (per test-integrity discipline)

## Provenance

Audit findings filed at commit `89df77c` via
`/agile-workflow:e2e-test-design --audit`. All child stories retain
their `e2e-audit-*` slug prefix to preserve provenance — these came
from an audit, not from feature-design.

## Next

`/agile-workflow:epic-design epic-e2e-playground-coverage` to refine
the decomposition (or skip directly to
`/agile-workflow:e2e-test-design feature-e2e-playground-coverage-golden`
to start designing the golden feature, since the decomposition is
already pre-clustered by the audit).
