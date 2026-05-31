---
id: epic-bug-squash-frontend-ws-lifecycle
kind: feature
stage: review
tags: [bug, ui]
parent: epic-bug-squash
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
---

# Frontend WebSocket connection lifecycle

## Brief

The SPA's WebSocket manager (`frontend/src/lib/ws.svelte.ts`) has three defects
in one connection-lifecycle surface: the per-session socket is never torn down
(consumers only remove a handler; `close()` is dead code in production, leaking
a live socket + reconnect machinery per visited session), `open()` overwrites an
in-reconnect record and resets the `lastSeenSeq` replay cursor (missed events +
zombie reconnect timer), and `subscribe()` floats `open()`'s rejection (a
silently dead subscription with no surfaced status).

This feature delivers a correct connection lifecycle: reference-counted teardown
so the socket closes when its last handler is removed, a reconnect-aware `open()`
that reuses an existing record without zeroing the replay cursor, and a
`subscribe()` that surfaces open failures into status instead of floating a
rejection. A single lifecycle rework resolves all three. It covers the
`ws.svelte.ts` manager only; it does NOT change the event-envelope schema or the
server-side gateway.

This feature is the **foundation for `epic-bug-squash-frontend-async-races`** —
the SessionList/component fixes there build on the corrected subscribe/close
contract, so it lands first.

## Epic context
- Parent epic: `epic-bug-squash`
- Position in epic: foundation frontend feature — `frontend-async-races` depends
  on its corrected `subscribe`/`close` contract.

## Foundation references
- `docs/SPEC.md` — WebSockets via coder/websocket, EventEnvelope spec-driven types
- Patterns: `wrapper-object-rune-store`, `openapi-fetch-middleware-client`

## Child stories (pre-existing, from bug-scan — re-parented here)
- `bug-squash-ws-connection-never-closed` — Medium, resource-leak — `frontend/src/lib/ws.svelte.ts:317`
- `bug-squash-ws-reconnect-cursor-reset` — Medium, async — `frontend/src/lib/ws.svelte.ts:248`
- `bug-squash-subscribe-floats-open-rejection` — Low, async — `frontend/src/lib/ws.svelte.ts:299`

## Architectural choice

**One coordinated lifecycle rework of `ws.svelte.ts`** — the three defects share
the connection-record lifecycle, so they're designed and implemented together
(single file; `implement-orchestrator` bundles them into one worktree). Three
seams: a reference-counted teardown, a reconnect-aware `open()`, and a
non-throwing `open()`.

## Implementation Units

### Unit 1: Reference-counted teardown (connection leak)
**File**: `frontend/src/lib/ws.svelte.ts`
**Story**: `bug-squash-ws-connection-never-closed` (Medium)

`subscribe`'s returned closure only removes one handler; the socket + record +
reconnect timer leak because `close(sessionId)` is never called in production.
Make the last unsubscribe tear the connection down — with a brief linger so a
synchronous unsubscribe-then-resubscribe (Svelte effect re-run) doesn't thrash:

```ts
function handlerCount(sessionId: string): number {
  const byType = handlers.get(sessionId);
  if (!byType) return 0;
  let n = 0; for (const set of byType.values()) n += set.size; return n;
}

// unsubscribe closure:
return () => {
  byType.get(type)?.delete(handler);
  if (handlerCount(sessionId) === 0) scheduleTeardown(sessionId);
};

// scheduleTeardown: defer one macrotask; cancel if a new subscribe arrives.
function scheduleTeardown(sessionId: string): void {
  const rec = records.get(sessionId);
  if (!rec) { handlers.delete(sessionId); setStatus(sessionId, null); return; }
  rec.teardownTimer ??= setTimeout(() => {
    rec.teardownTimer = null;
    if (handlerCount(sessionId) === 0) teardown(sessionId); // still nobody listening
  }, 0);
}
// subscribe(): on entry, cancel any pending teardown for this session.
```

`teardown(sessionId)` is the connection-closing body of today's `close()`
(close ws, clear reconnect timer, delete record + handlers entry, clear status).
`close()` becomes `cancel pending teardown → teardown()` so the public
cursor-invalidation semantics are unchanged.

**Implementation Notes**: add `teardownTimer` to `ConnectionRecord`. The linger
(macrotask `setTimeout(...,0)`) absorbs the synchronous effect
cleanup→re-subscribe window — codex verified against the Svelte 5 effect docs
that effects re-run in a microtask with teardown immediately before the rerun,
so a macrotask timer cannot fire between cleanup and the synchronous rerun body.
Tearing down genuinely drops `lastSeenSeq` (intended "left the view" cursor
invalidation, per the existing `close()` doc). Also prune the empty per-type
`Set` on unsubscribe so long-lived churn across event types doesn't accumulate
empty sets.

**Acceptance Criteria**:
- [ ] Unsubscribing the LAST handler for a session closes the ws, clears the
      record/timer/status (no leak); unsubscribing one of several does not.
- [ ] A synchronous unsubscribe-all → resubscribe (effect re-run) does NOT close
      the socket (teardown lingered + cancelled).

### Unit 2: Reconnect-aware open() (cursor reset + orphaned timer)
**File**: `frontend/src/lib/ws.svelte.ts`
**Story**: `bug-squash-ws-reconnect-cursor-reset` (Medium)

When `open()` runs while a record exists but `ws === null` (mid-reconnect), the
`if (existing && existing.ws)` guard is false, so it overwrites the record with a
fresh `lastSeenSeq: 0` and orphans the pending reconnect timer. Reuse any
existing record:

```ts
async function open(sessionId: string): Promise<WebSocket | null> {
  const existing = records.get(sessionId);
  if (existing) return existing.ws; // open OR mid-reconnect — reconnect loop owns it; do NOT overwrite

  if (!auth.token) { setStatus(sessionId, null); return null; } // Unit 3
  const ticket = await fetchTicket();
  // POST-TICKET GUARD (codex must-fix): a teardown/close/unsubscribe may have
  // landed during the await. Re-check BEFORE constructing the socket.
  if (!ticket) return null;
  const now = records.get(sessionId);
  if (now) return now.ws;                       // someone created/owns a record meanwhile
  if (handlerCount(sessionId) === 0) return null; // last handler left during the await — don't orphan a socket
  // ...only now create the fresh record + WebSocket...
}

// reopen(): identity-guard every async gap (codex must-fix)
async function reopen(sessionId: string): Promise<void> {
  const rec = records.get(sessionId);
  if (!rec) return;
  const ticket = await fetchTicket();
  if (!ticket) { /* drop as today */ return; }
  // require the SAME record AND live handlers — a teardown+fresh-subscribe may
  // have replaced rec during the await; an old reopen must not attach an orphan.
  if (records.get(sessionId) !== rec || handlerCount(sessionId) === 0) return;
  // ...construct socket, attachListeners...
}
```

**Implementation Notes**: a lingering record always means "open or reconnecting"
(the close handler deletes the record on a non-reconnectable/closedByUs close),
so returning `existing.ws` (possibly null) without recreating preserves
`lastSeenSeq` and the in-flight `reconnectTimer`. **Codex async-cancellation
must-fixes**: (a) `open()` must re-check after `fetchTicket()` and abort
(return null, create nothing) if the record reappeared OR all handlers left;
(b) `reopen()` must verify `records.get(sessionId) === rec` (identity, not just
existence) and `handlerCount > 0` after its await, else an in-flight reopen
attaches an orphan socket after a teardown + fresh resubscribe.

**Acceptance Criteria**:
- [ ] Calling `subscribe`/`open` for a session whose record is mid-reconnect
      (ws===null, timer pending) does NOT reset `lastSeenSeq` and does NOT spawn
      a second reconnect timer.

### Unit 3: open() returns null instead of throwing (floated rejection)
**File**: `frontend/src/lib/ws.svelte.ts`
**Story**: `bug-squash-subscribe-floats-open-rejection` (Low)

`open()` throws on `!auth.token`; `subscribe` calls `void open(...)`, so the
rejection is floated (unhandled) and the handler is registered against a socket
that never opens. Make `open()` resolve `null` (consistent with its
ticket-failure path) and surface a disconnected status:

```ts
if (!auth.token) {
  setStatus(sessionId, null); // disconnected — no token yet
  return null;
}
```

**Implementation Notes**: `void open(sessionId)` in `subscribe` is then safe (no
rejection). The handler stays registered; if a later subscribe occurs once a
token is present, the socket opens and handlers fire. WsStatus has no 'failed'
member, so null (disconnected) is the honest state — not worth widening the union
for this Low. Anonymous/pre-token subscribe → disconnected until a token exists
(documented limitation; the reconnect loop handles network drops, not initial
no-token).

**Acceptance Criteria**:
- [ ] `open()` with no auth token resolves `null` (no throw / no unhandled
      rejection); status is null; no socket/record is created.

## Implementation Order
Single file — one coordinated change. No story-level `depends_on`;
`implement-orchestrator` MUST bundle the 3 into one worktree (same file). This
whole feature is a prerequisite for `epic-bug-squash-frontend-sessionlist-subscription`
(declared at the feature level).

## Testing (vitest + jsdom, existing ws.test.ts patterns)
- Unit 1: mock `WebSocket`; subscribe×2 then unsubscribe both → `ws.close()`
  called + record gone; unsubscribe one → still open; synchronous
  unsubscribe-all→resubscribe → not closed (linger cancelled).
- Unit 2: drive a record into mid-reconnect (close event with a reconnectable
  code, ws=null, timer pending); call subscribe → assert `lastSeenSeq` preserved
  and only one timer.
- Unit 3: stub `auth.token` falsy; `open()` resolves null, no throw (assert no
  unhandled rejection), no record created. Also assert the documented limitation:
  a pre-token handler stays registered but does NOT auto-open when a token later
  appears (a new subscribe/open is required).
- Async cancellation (codex must-fix): unsubscribe/close WHILE `fetchTicket()` is
  in flight → `open()`/`reopen()` create no socket (mock a deferred ticket
  promise; tear down before it resolves; assert no `WebSocket` constructed).

## Risks
- **Linger duration**: `setTimeout(0)` absorbs the synchronous effect re-run; if
  some consumer unsubscribes and re-subscribes across a real async gap > one
  macrotask, the socket closes and re-opens (fresh cursor) — acceptable
  ("left and came back"). The SessionList churn that would stress this is fixed
  in the dependent feature.
- **Cursor loss on teardown is intended** — matches the existing `close()`
  contract; not a regression.

## Design decisions
- **Ref-count + linger** over "never auto-close" (keeps the leak) or
  "close immediately at zero" (thrashes on effect re-runs). The macrotask linger
  is the minimal anti-thrash measure.
- **open() reuses any existing record** over the current overwrite — a lingering
  record is always open-or-reconnecting; recreating it is what causes the bug.
- **open() returns null, no 'failed' status** over widening WsStatus — the Low
  doesn't justify a union change touching all consumers.

## Other agent review

Codex (cross-model, xhigh) reviewed this design. Verdict: approve with the async
cancellation guards as must-fix. Confirmed non-issues: `setTimeout(0)` linger IS
sufficient (verified against Svelte 5 `$effect` docs — teardown runs immediately
before the synchronous rerun, so a macrotask can't fire between); no page-unload
leak; `if (existing) return existing.ws` is correct across all states once the
reopen identity guard exists; `open()` is private so returning null (vs throwing)
is the right contract repair.

**Accepted & applied:**
- **Unit 1 (async leak)**: `open()` now re-checks AFTER `fetchTicket()` — if a
  record reappeared, reuse it; if `handlerCount === 0`, return null and create no
  socket (handles unsubscribe/close during the ticket await).
- **Unit 1+2 (orphan reopen)**: `reopen()` now requires `records.get(id) === rec`
  (identity, not existence) and `handlerCount > 0` after its await, so an
  in-flight reopen can't attach an orphan socket after a teardown + fresh
  resubscribe.
- **Unit 1 (nice-to-have)**: prune empty per-type handler sets on unsubscribe.
- **Unit 3 (nice-to-have)**: test the documented late-token limitation.

## Implementation summary

All 3 child stories implemented and advanced to `stage: review` (per-story `implement: bug-squash-*` commits). Each landed a failing-first regression test; the codex feature-gate findings (see `## Other agent review`) were applied during design and honored in implementation. Verification at the orchestrator level: `go build ./...` + `go vet` clean; backend `-race`/package tests and frontend `vitest` (764 passing) + `svelte-check` green; `sqlc generate` matches spec.
