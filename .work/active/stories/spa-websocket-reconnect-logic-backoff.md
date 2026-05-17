---
id: spa-websocket-reconnect-logic-backoff
kind: story
stage: review
tags: [ui]
parent: spa-websocket-reconnect-logic
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# SPA WS — exponential backoff reconnect loop + close-code predicate + status rune

## Scope

Refactor `frontend/src/lib/ws.svelte.ts` so the existing per-session
socket map carries a richer per-session **connection record** with
status and reconnect bookkeeping, and add the reconnect loop that
fires on unexpected close. Replay-from-seq is **not** part of this
story (next story); this one re-opens with no replay frame so we can
land the loop and tests independently.

## Files touched

- `frontend/src/lib/ws.svelte.ts` (edit) — refactor `sockets` map into
  a `records` map of `{ ws, status, attempt, closedByUs }`; add
  reconnect loop on `close`; add `shouldReconnect(code)` predicate;
  add the `wsStatus` rune store (wrapper-object pattern, mirrors
  `auth.svelte.ts`).
- `frontend/src/lib/ws.test.ts` (edit) — extend `MockWebSocket` to
  carry a `code` on `close`; add tests covering the loop and
  predicate. Use `vi.useFakeTimers()` for delay assertions.

## Specification

### Connection record (internal)

```ts
type WsStatus = 'connecting' | 'open' | 'reconnecting';

interface ConnectionRecord {
  ws: WebSocket | null;
  status: WsStatus;
  attempt: number;       // 0 when open, increments per backoff tick
  closedByUs: boolean;
  reconnectTimer: ReturnType<typeof setTimeout> | null;
}
```

### Constants

```ts
const RECONNECT_BASE_MS = 1000;
const RECONNECT_CAP_MS = 30_000;
const RECONNECT_MULT = 1.6;
const RECONNECT_JITTER = 0.25; // ±25%
```

### `shouldReconnect(code: number): boolean`

Returns `false` for: `1000`, `1003`, `1007`, `1008`, any code in
`[4000, 5000)`. Returns `true` otherwise (covers `1001`, `1006`,
`1011`, `1012`, `1013`, `1014`, and the no-code-supplied path which
browsers normalise to `1006`).

### Reconnect loop

In the `'close'` listener:

```ts
ws.addEventListener('close', (ev: CloseEvent) => {
  const rec = records.get(sessionId);
  if (!rec) return;
  rec.ws = null;
  if (rec.closedByUs || !shouldReconnect(ev.code)) {
    records.delete(sessionId);
    setStatus(sessionId, null); // banner consumers see "no status" → no banner
    return;
  }
  rec.status = 'reconnecting';
  setStatus(sessionId, 'reconnecting');
  const delay = backoffDelay(rec.attempt);
  rec.reconnectTimer = setTimeout(() => {
    rec.attempt += 1;
    reopen(sessionId);
  }, delay);
});
```

`backoffDelay(attempt)` returns
`min(BASE * MULT^attempt, CAP)` multiplied by a uniform random factor
in `[1 - JITTER, 1 + JITTER]`. Tests pin `Math.random` via
`vi.spyOn(Math, 'random').mockReturnValue(0.5)` so the jittered value
is deterministic (factor = 1.0).

### `wsStatus` rune store

Wrapper-object pattern (mirrors `auth.svelte.ts:21`):

```ts
const _statuses = $state<Record<string, WsStatus | null>>({});

export const wsStatus = {
  for(sessionId: string): WsStatus | null {
    return _statuses[sessionId] ?? null;
  },
};

function setStatus(sessionId: string, s: WsStatus | null) {
  _statuses[sessionId] = s;
}
```

`'open'` listener sets `'open'`, the reconnect path sets
`'reconnecting'`, and explicit `close(sessionId)` sets `null`. The
initial `open()` call sets `'connecting'`.

### `close(sessionId)` semantics

`close()` sets `closedByUs = true`, clears any pending
`reconnectTimer`, calls `ws.close()`, deletes the record, and clears
the status rune for this session. This is the cursor-invalidation
path (covered by the next story).

## Acceptance criteria

- [ ] `shouldReconnect(code)` returns `false` for `1000`, `1003`,
      `1007`, `1008`, and any code in `[4000, 5000)`; returns `true`
      for `1001`, `1006`, `1011`, `1012`, `1013`, `1014`.
- [ ] Unexpected close (code 1006) schedules a reconnect with delay
      `~1000ms` on first attempt, `~1600ms` on second, capped at
      30 000 ms.
- [ ] `wsStatus.for(sessionId)` returns `'connecting'` before the
      first open, `'open'` after, `'reconnecting'` during a backoff
      wait, and `null` after explicit `close()`.
- [ ] Calling `close(sessionId)` while a reconnect timer is pending
      cancels the timer and stops the loop.
- [ ] All existing `ws.test.ts` tests still pass without behavioural
      change.
- [ ] At least one new test asserts the reconnect timer fires and
      opens a fresh `MockWebSocket` instance.

## Test approach

`vi.useFakeTimers()` advances the clock past `backoffDelay(0)` and
asserts a new `MockWebSocket` instance is created.
`vi.spyOn(Math, 'random').mockReturnValue(0.5)` removes jitter
variance so delays are exact. Close-code assertions emit a
`CloseEvent` with a numeric `code` via the extended
`MockWebSocket.emit('close', code)` helper.

## Notes

- The `MockWebSocket.emit('close')` helper currently doesn't carry a
  code; this story extends it (a pure test-fixture refactor) — covered
  by the test integrity rule, not a bug.
- No production code reaches the cursor / replay-from frame yet — that
  layer lands in the next story.
- The reconnect loop is per-session; two sessions reconnect
  independently with independent attempt counters.

## Implementation notes

**Files touched**

- `frontend/src/lib/ws.svelte.ts` — refactored from a bare `sockets:
  Map<string, WebSocket>` to a `records: Map<string, ConnectionRecord>`
  holding `{ ws, status, attempt, closedByUs, reconnectTimer }`. Added
  `shouldReconnect(code)` (exported for test coverage), `backoffDelay(attempt)`
  (private — `min(BASE * MULT^attempt, CAP) * jitterFactor`), and the
  `wsStatus` rune store using the wrapper-object pattern from
  `auth.svelte.ts`. The `'open'` listener resets `attempt = 0` and sets
  status `'open'`; the `'close'` listener decides reconnect-or-drop via
  the predicate and schedules a `setTimeout`. `close(sessionId)` cancels
  any pending timer and clears the status to `null`.

- `frontend/src/lib/ws.test.ts` — extended `MockWebSocket.emit('close',
  code?)` to carry a numeric close code (defaults to `1006` when omitted,
  matching browser behaviour for transport tears). Added an
  `emit('open')` helper so status-rune tests can drive the `'open'`
  transition. Updated the pre-existing "socket close event causes a new
  socket on next subscribe" test to use code `1000` (clean close → no
  reconnect) so its semantics match the new behaviour. Added three new
  describe blocks — `shouldReconnect predicate` (3 tests), `reconnect
  loop / exponential backoff` (7 tests), `wsStatus rune store` (5
  tests) — for 27 tests total in the file.

**Decisions within the design envelope**

- Exported `shouldReconnect` so the predicate can be unit-tested
  directly. The design called for "a small pure function so each branch
  can be unit-tested without spinning up a real socket"; exporting was
  the cleanest way to satisfy that without test-double gymnastics.
- `setStatus(sessionId, null)` deletes the key from the `_statuses`
  rune state rather than storing `null`. Both shapes satisfy
  `wsStatus.for() ?? null`, but deletion keeps the state object clean
  and avoids growing it across the lifetime of the SPA.
- `reopen()` re-reads `auth.token` at reconnect time. If the token has
  been cleared (signOut while a reconnect was pending), the record is
  dropped silently — the auth layer will redirect to login anyway, so
  this is the right default.
- The reconnect path in the test that asserts the second backoff
  delay (~1600ms) drains the timer naturally; subsequent `emit('close',
  1006)` on the freshly-opened socket increments the per-record
  `attempt` counter via the existing scheduling path.

**Test results**

`cd frontend && npm test -- --run src/lib/ws.test.ts` — **27/27 pass**.
Full suite (`npm test -- --run`) — **320/320 pass**.

`npm run check` — six pre-existing errors in
`src/lib/components/finalize/RefGroupList.test.ts` (unrelated `Set<unknown>` vs
`Set<string>` typing); confirmed they reproduce on `main` without these
changes. Zero new errors or warnings introduced.

**Deviations from design**

None. The story spec is implemented as written; the parent feature's
broader scope (replay-from cursor, status banner component, e2e
helpers) is deferred to the sibling stories as required.
