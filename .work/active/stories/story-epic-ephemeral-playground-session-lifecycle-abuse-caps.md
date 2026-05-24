---
id: story-epic-ephemeral-playground-session-lifecycle-abuse-caps
kind: story
stage: implementing
tags: [portal, playground, security]
parent: feature-epic-ephemeral-playground-session-lifecycle
depends_on: [story-epic-ephemeral-playground-session-lifecycle-rest-endpoints]
release_binding: null
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
