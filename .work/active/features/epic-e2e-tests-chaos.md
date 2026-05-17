---
id: epic-e2e-tests-chaos
kind: feature
stage: drafting
tags: [e2e-test, testing]
parent: epic-e2e-tests
depends_on: [epic-e2e-tests-golden-path]
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# E2E Tests — Chaos

## Brief

Chaos scenarios verify the graceful-degradation behaviors jamsesh
already advertises: the retry queue in plugin hooks
(`epic-cc-plugin-hooks-retry-queue-and-simple-hooks`), the
push-per-commit retry semantics, the auto-merger's conflict-fallback
behavior, and the SPA's WebSocket reconnect logic. Chaos depends on
`golden-path` because we verify graceful degradation of paths
golden has already proven work end-to-end — a chaos test against an
unverified golden path is debugging two things at once.

## Scope

### Chaos scenarios (Go specs)

1. **`network_jitter_db.spec`** — Toxiproxy injects 500ms latency +
   10% packet loss between portal and Postgres while a session has
   active pushes. Invariant: pushes either succeed (with elevated
   latency) or surface a clear 503 with a Retry-After header; no
   partial-state writes (verified by a post-chaos snapshot of the
   sessions / events / refs tables).

2. **`automerger_pause.spec`** — Pumba pauses the portal container
   for 5 seconds while a sync-ref push is mid-merge. Invariant: when
   the container resumes, the push completes and the auto-merger
   advances draft; no `conflict.detected` event spuriously fires.

3. **`clock_skew_token_expiry.spec`** — libfaketime shifts the
   portal's clock forward by 1 hour mid-session. Invariant: bearer
   tokens issued before the shift continue to validate until their
   actual exp; tokens issued under the shifted clock honor the
   shifted exp; the SPA's refresh-token flow recovers without the
   user re-logging-in.

4. **`ws_reconnect_drop.spec`** — Toxiproxy drops the WebSocket
   connection mid-event burst (5 events in flight). Invariant: the
   SPA reconnects within 2s, replays from cursor, and renders all 5
   events in order. Driven by Playwright + a backend event emitter
   probe.

5. **`oauth_provider_timeout.spec`** — Toxiproxy adds 10s latency to
   the mock-oauth2-server response. Invariant: the magic-link / OAuth
   flow either succeeds (within the portal's configured timeout) or
   surfaces a clear `auth.provider_unavailable` error; no half-issued
   tokens.

### Anti-tautology guardrails

Every chaos scenario:

- Asserts on a user-visible outcome AFTER chaos clears (state of the
  system as a human/agent would observe it next)
- States its graceful-degradation invariant in plain English
- Has a paired "without chaos, this passes" assertion run BEFORE the
  fault is injected (proves the test isn't accidentally green for the
  wrong reason)

## Out of scope

- Fuzzed inputs (fuzzing feature)
- Static failure inputs / boundary values (failure-mode feature)
- Chaos coverage of subsystems that don't yet have retry/fallback
  behavior — if discovery finds a graceful-degradation gap (e.g., no
  WS reconnect in the SPA yet), the design pass on this feature
  spawns a back-pressure item to the relevant epic rather than
  writing a tautology

## Foundation references

- `docs/ARCHITECTURE.md > Recovery` — the failure modes this feature
  exercises
- `docs/PROTOCOL.md > Error responses` — clear-error contract
- `.work/active/features/epic-cc-plugin-hooks-retry-queue-and-simple-hooks.md`
  — retry queue invariants
- `.work/active/epics/epic-e2e-tests.md` — parent mock policy (Toxiproxy +
  Pumba + libfaketime in the catalog)

## Acceptance criteria

- [ ] 5 Go chaos specs green
- [ ] Each spec includes the paired before-chaos assertion proving the
      test isn't a tautology
- [ ] Each spec's graceful-degradation invariant is stated in plain
      English in a top-level doc comment
- [ ] The Playwright WS-reconnect scenario runs in headless Chromium
      with deterministic timing (no `sleep`-based waits; only
      condition-driven waits)
- [ ] No scenario passes by skipping — `t.Skip` is treated as a
      failure in this suite's CI runner
