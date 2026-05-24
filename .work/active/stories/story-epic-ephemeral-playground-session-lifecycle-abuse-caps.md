---
id: story-epic-ephemeral-playground-session-lifecycle-abuse-caps
kind: story
stage: done
tags: [portal, playground, security]
parent: feature-epic-ephemeral-playground-session-lifecycle
depends_on: [story-epic-ephemeral-playground-session-lifecycle-rest-endpoints]
release_binding: v0.4.0
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# Playground abuse caps (rate-limit + pre-receive)

## Scope

Story 3 of the parent feature. Wires three abuse caps:
1. **Per-IP create rate limit** on `POST /api/playground/sessions` —
   uses existing `internal/portal/ratelimit` infrastructure with a
   playground-specific Config
2. **Per-session push throughput cap** at `pre-receive` — rejects pushes
   that would exceed `JAMSESH_PLAYGROUND_MAX_CONTENT_BYTES` accumulated
   for the session
3. **Activity-reset wiring** — after each substantive event (commit
   pushed, comment posted, finalize-attempt POSTed), update the
   session's `last_substantive_activity_at` so the destruction worker
   reads accurate idle state

The per-session max-participants cap is owned by Story 1 (it's enforced
in the JoinSession handler). This story owns the rate-limit + pre-receive +
activity-reset concerns.

Full design in the parent feature body's "Story 3" section.

## Files delivered

- `internal/portal/playground/ratelimit.go` — NewCreateRateLimiter +
  RateLimitMiddleware
- `internal/portal/playground/ratelimit_test.go`
- `internal/portal/prereceive/playground_caps.go` — CheckPlaygroundCaps,
  called from existing `Validator.Validate` when org_id matches the
  reserved playground org
- `internal/portal/prereceive/playground_caps_test.go`
- `internal/portal/router/router.go` (modify) — mount rate-limit
  middleware on `POST /api/playground/sessions` only
- Activity-reset call-site additions in:
  - `internal/portal/githttp/receive_pack.go` (post-receive commit handler)
  - `internal/portal/comments/handler.go` (comment-create handler)
  - `internal/portal/sessions/handler.go` (finalize-attempt handler — wherever it lives)
- `db/queries/{sqlite,postgres}/sessions.sql` (extend) —
  `ResetSessionIdleTimer` (UPDATE both columns in one statement),
  `SumPushedBytesLastHour`

## Acceptance criteria

See the parent feature body's "Story 3 acceptance criteria" section.
Summary: 4th create from same IP within an hour → 429 + Retry-After;
pre-receive rejects size-exceeded pushes with `remote: ERROR:
playground.size_exceeded`; substantive events reset the idle timer;
durable sessions unaffected.

## Notes for the implementing agent

- The existing `internal/portal/ratelimit` package uses
  `Config{PerMinute: N}` as its unit. Convert per-hour cap to
  per-minute: `(cfg.CreatePerIPHour + 59) / 60` (round up, min 1).
- Mount the rate-limit middleware **only on the playground create
  route**, not on join or status. Joiners aren't rate-limited (they
  can't easily DoS — joining requires a session ID, and the session
  has a participant cap).
- `Validator.Validate` extension: call `CheckPlaygroundCaps` as the
  LAST check (after ref / scope / trailer validation). Early returns
  on the existing checks short-circuit before the new code runs, so
  durable session pushes are unaffected.
- Activity-reset is a single SQL UPDATE that bumps both
  `last_substantive_activity_at` and `idle_timeout_at` (= now +
  IdleTimeout). The reset is best-effort: if the UPDATE fails (e.g.
  transient DB error), log a warning and continue — the destruction
  worker will catch the session eventually based on the previous
  timer value, just slightly earlier than expected. Not worth
  failing the substantive event itself.
- For the `SumPushedBytesLastHour` query, you'll need to track
  per-push byte counts somewhere. Two options:
  (A) Add a `push_events` table that records each push's byte size +
      timestamp; sum over rolling window.
  (B) Track total content size on the session row directly; reject
      based on total (simpler, more conservative — once exceeded,
      session permanently can't receive more pushes).
  Recommend (B) for simplicity and because the locked decision says
  the cap is on TOTAL content, not rate. The "rolling hour" was a
  red herring in the parent feature body's first draft — fix the
  parent body to clarify if you go with (B).

## Implementation notes

Implemented 2026-05-23. Chose Option B (total content size cap) per
the story guidance.

### Design decisions made

**Content-size check via disk-walk**: `CheckPlaygroundCaps` measures the
current on-disk repo size by walking the bare repo directory
(`filepath.WalkDir`). No new DB query or push-event table needed. Consistent
with Option B: the cap is on TOTAL accumulated content, not a rolling window.
When the repo path is empty or non-existent (first push, or walk error), size
is treated as 0 so the first push is never rejected by the size check alone.

**Per-minute burst = ceil(CreatePerIPHour/60), min 1**: With the default
`CreatePerIPHour=3`, `perMinute=1`. This is stricter than "3 per hour" — the
minute-level burst of 1 prevents rapid-fire abuse (you can't fire 3 creates in
1 second). The hourly cap of 3 prevents accumulation over time. Both limiters
must pass.

**`RepoPath` added to `ValidateInput`**: Added an optional `RepoPath string`
field to `prereceive.ValidateInput`. When empty, the playground size check
treats current size as 0 (forward-compatible). `receive_pack.go` passes
`repoPath` which is already computed at that point.

**`PlaygroundIdleTimeout` threaded via local constants**: Each package that
needs the playground org_id check defines its own `const playgroundOrgID =
"org_playground"` to avoid import cycles (githttp → playground, comments →
playground, sessions → playground are all cyclic).

**Activity-reset wired in 3 call-sites**:
1. `internal/portal/githttp/receive_pack.go` — after successful push and storage sync
2. `internal/portal/comments/service.go` — after successful comment insert (TX + fanout)
3. `internal/portal/sessions/handler.go` — after successful active→finalizing transition

All three use the same best-effort pattern: log a warning on reset failure, do
not fail the substantive event.

**`WithPlaygroundIdleTimeout` builder**: `sessions.Handler` exposes a
`WithPlaygroundIdleTimeout(d) *Handler` method so main.go can wire the timeout
without changing the constructor signature (preserves backward compat with tests
that construct Handler directly).

### Files delivered

- `internal/portal/playground/ratelimit.go` (new)
- `internal/portal/playground/ratelimit_test.go` (new)
- `internal/portal/prereceive/playground_caps.go` (new)
- `internal/portal/prereceive/playground_caps_test.go` (new)
- `internal/portal/prereceive/validate.go` (modified — added `PlaygroundMaxContentBytes`, `Logger` to `Validator`; wired `CheckPlaygroundCaps` as last check)
- `internal/portal/prereceive/types.go` (modified — added `RepoPath` to `ValidateInput`)
- `internal/portal/githttp/handler.go` (modified — added `PlaygroundIdleTimeout`, `playgroundOrgID`)
- `internal/portal/githttp/receive_pack.go` (modified — passes `RepoPath`; activity-reset after successful push)
- `internal/portal/playground/handler.go` (modified — added `CreatePerIPHour`, `MaxContentBytes` to `Config`)
- `internal/portal/comments/service.go` (modified — added `PlaygroundIdleTimeout`; activity-reset after comment create)
- `internal/portal/sessions/handler.go` (modified — added `playgroundIdleTimeout`, `WithPlaygroundIdleTimeout`, activity-reset in FinalizeSession)
- `cmd/portal/main.go` (modified — threads pgCfg to all consumers; mounts `pgCreateRL` on playground create route; wires `PlaygroundMaxContentBytes` to Validator)

SQL queries (`ResetSessionIdleTimer`, `ListExpiredPlaygroundSessions`) were
already present from prior story implementations — no SQL changes needed.

## Review (2026-05-23)

**Verdict**: Approve with comments

**Blockers**: none

**Important**:
- Activity-reset call-sites lack coverage — `idea-playground-abuse-caps-activity-reset-integration-test`
- `comments/service.go` uses stdlib `log` instead of project-standard `slog` — `idea-comments-service-use-slog-not-stdlib-log`

**Nits**:
- `CheckPlaygroundCaps` return signature `(Rejection, bool)` where `true` means
  "allowed/skip" is mildly unusual; reads correctly at the call site but a future
  reader may briefly invert the meaning. Not worth a separate item.
- Four packages (githttp, comments, sessions, prereceive) each define a local
  `playgroundOrgID = "org_playground"` const to dodge import cycles. A shared
  `internal/portal/playground/orgid` package exporting just the constant would
  collapse this — small refactor, not blocking.
- The disk-walk `repoDirSize` counts every byte under the bare repo dir
  (objects, refs, hooks, config). Conservative against the cap; acceptable for
  playground where the cap (50 MiB default) has comfortable margin.

**Notes**:
- `go build ./...` clean; `go vet` clean on touched packages; tests pass for
  `playground`, `prereceive`, `comments`, `githttp`, `sessions`, `ratelimit`.
- Pre-receive durable-session pass-through is well-covered by the integration
  test `TestValidate_DurableSessionUnaffectedByPlaygroundCap`.
- Rate-limit per-IP isolation and 4th-create-blocked acceptance criteria are
  both directly tested.
- chi RealIP middleware is correctly wired via the router when
  `TrustProxyHeaders` is enabled — per-IP keying works behind a proxy.
- Sibling stories under the parent feature are all `done`; parent feature was
  already at `stage: review` so no parent-advance needed.
