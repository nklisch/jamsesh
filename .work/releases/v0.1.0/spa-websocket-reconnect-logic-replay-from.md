---
id: spa-websocket-reconnect-logic-replay-from
kind: story
stage: done
tags: [ui]
parent: spa-websocket-reconnect-logic
depends_on: [spa-websocket-reconnect-logic-backoff]
release_binding: v0.1.0
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# SPA WS — lastSeenSeq cursor + replay_from frame on reconnect

## Scope

Add a `lastSeenSeq` cursor to the per-session connection record. The
message handler advances the cursor when a parsed envelope's `seq`
exceeds the current value. On reconnect (via the backoff loop landed
in `spa-websocket-reconnect-logic-backoff`), the new socket's
`'open'` listener writes `{"replay_from": <lastSeenSeq>}` as the
first text frame when `lastSeenSeq > 0`. The portal then streams
events with `seq > lastSeenSeq` before transitioning to live.

## Files touched

- `frontend/src/lib/ws.svelte.ts` (edit) — extend the connection
  record with `lastSeenSeq: number` (default 0); advance it in the
  `'message'` handler; emit the replay frame in the `'open'` listener
  when both `lastSeenSeq > 0` and this is a reconnect (i.e. the
  record already existed).
- `frontend/src/lib/ws.test.ts` (edit) — add tests covering cursor
  advancement, replay-frame emission on reconnect, and cursor reset
  on explicit `close()`.

## Specification

### Cursor data flow

```ts
interface ConnectionRecord {
  // ... existing fields from -backoff story ...
  lastSeenSeq: number;     // 0 = no events seen yet
}
```

In the `'message'` handler:

```ts
const env = JSON.parse(ev.data as string) as EventEnvelope;
const rec = records.get(sessionId);
if (rec && typeof env.seq === 'number' && Number.isFinite(env.seq) && env.seq > rec.lastSeenSeq) {
  rec.lastSeenSeq = env.seq;
}
```

The cursor only moves forward. Out-of-order delivery (replayed events
arriving after live events that already moved the cursor — unlikely
given the portal sends replay before live, but defensive) is a no-op.

### Replay frame on reconnect

In the `'open'` listener, after setting `status = 'open'` and
resetting `attempt`:

```ts
// Only send replay_from on a RECONNECT (lastSeenSeq > 0).
// On a fresh subscribe, no frame is sent.
if (rec.lastSeenSeq > 0) {
  ws.send(JSON.stringify({ replay_from: rec.lastSeenSeq }));
}
```

Per `internal/portal/wsgateway/gateway.go:213`, the portal reads at
most one text frame, expects exactly the `{"replay_from": N}` shape,
and replays events with `seq > N`. No-frame and zero-value first
frames are both handled as "no replay" by the portal (the 2-second
timer falls through).

### Cursor invalidation on `close(sessionId)`

`close()` already deletes the record (per the `-backoff` story),
which discards `lastSeenSeq` along with it. A subsequent
`subscribe()` starts with `lastSeenSeq = 0`, sends no replay frame,
and gets a pure live stream.

## Acceptance criteria

- [ ] The `'message'` handler advances `lastSeenSeq` when a parsed
      envelope carries a numeric `seq` greater than the current
      cursor.
- [ ] Out-of-order or duplicate seqs do not move the cursor
      backwards.
- [ ] Envelopes without a `seq` field (or with non-numeric `seq`) do
      not move the cursor and do not throw.
- [ ] After an unexpected close that triggers reconnect, the new
      socket receives a single first-frame write of
      `{"replay_from": <lastSeenSeq>}` when `lastSeenSeq > 0`.
- [ ] When `lastSeenSeq == 0` (no events seen pre-drop), the
      reconnect opens a fresh stream with NO replay frame.
- [ ] Explicit `close(sessionId)` resets the cursor; a subsequent
      subscribe opens with `lastSeenSeq = 0` and sends no replay
      frame.

## Test approach

Extend the existing `MockWebSocket` test fixture with a `sent: string[]`
array that captures every `send()` call. Drive a sequence:

1. Subscribe, fire `'open'`, emit messages with seqs `1, 2, 3`.
2. Fire a `close` with code 1006.
3. Advance the fake timer past `backoffDelay(0)`.
4. Assert a new `MockWebSocket` instance was created.
5. Fire `'open'` on the new instance.
6. Assert exactly one `send()` happened with body
   `{"replay_from":3}`.

A second test covers the fresh-subscribe path (no events seen): no
`send()` should happen after `'open'`.

## Notes

- The portal's existing replay support is the contract surface; this
  story doesn't change the wire protocol, only the client's use of
  it.
- Idempotency of replayed events is the consumers' problem.
  `ActivityFeed.svelte` already keys by `env.seq` in its
  `{#each events as env (env.seq)}` loop, so duplicates collapse.
  Other consumers — `TreeDag.svelte`, `CommentsTab.svelte` — need a
  spot-check during implementation; any required dedupe lands as a
  follow-up story (park if it surfaces, do not silent-fix).

## Implementation notes

Landed per the design with no deviations.

### Changes in `frontend/src/lib/ws.svelte.ts`

- Added `lastSeenSeq: number` to `ConnectionRecord` (plain field; not a
  rune, per D2). Initialized to `0` in `open()`.
- The `'message'` handler now looks up the record, and when the parsed
  envelope carries a finite numeric `seq` greater than the current
  cursor, advances `rec.lastSeenSeq`. Envelopes without `seq`,
  non-numeric `seq` (e.g. strings), or non-finite values (`NaN`,
  `Infinity`) are no-ops. Out-of-order / duplicate `seq` values do not
  move the cursor backwards.
- The `'open'` listener emits `JSON.stringify({ replay_from: <seq> })`
  via `ws.send()` immediately after setting `status = 'open'` and
  before any other writes, but only when `rec.lastSeenSeq > 0`. On
  fresh subscribes the listener writes nothing, matching the portal's
  2-second-falls-through "no replay" path.
- Module head comment updated to describe the now-current behavior
  (removing the "sibling story" forward-reference).
- `close(sessionId)` was left untouched — it drops the record, which
  takes `lastSeenSeq` down with it. The next subscribe starts fresh
  with `lastSeenSeq = 0` and sends no replay frame.

### Changes in `frontend/src/lib/ws.test.ts`

- Extended `MockWebSocket` with a `sent: string[]` array and replaced
  the no-op `send()` with one that pushes string frames onto it.
- Added a new `describe('ws — lastSeenSeq cursor + replay_from', ...)`
  suite with six tests covering: fresh-subscribe sends nothing,
  reconnect after seeing events sends `{"replay_from":<max>}` as the
  first frame, cursor-only-moves-forward (out-of-order / duplicate),
  malformed `seq` values are ignored without throwing, explicit
  `close()` invalidates the cursor (next subscribe sends no frame),
  and multi-reconnect chains continue to send the latest cursor.

### Test result

- `npm test -- --run src/lib/ws.test.ts`: **33/33 passing** (up from
  the prior 27; this story adds 6).
- `npm run check`: 6 pre-existing errors in
  `src/lib/components/finalize/RefGroupList.test.ts` (`Set<unknown>`
  vs `Set<string>`) — not in this story's scope. No new errors
  introduced.

### Surfaces deliberately left alone

Per the assignment, this story did not touch
`frontend/src/lib/components/`, `SessionViewShell.svelte`, or the
Playwright spec — those belong to the `-status-ui` sibling.

## Review (2026-05-17)

**Verdict: Approve.**

Implementation at `ad61c2a` matches the spec exactly. Cross-checked
against the substrate body and verified directly:

- `ConnectionRecord.lastSeenSeq: number` is a plain field (D2),
  initialized to `0` in `open()`.
- The `'message'` handler advances the cursor only when the parsed
  envelope's `seq` is a finite number greater than the current value
  — `typeof === 'number'` + `Number.isFinite` + `>` guards. Out-of-
  order / duplicate / missing / NaN / Infinity / string `seq` are all
  no-ops.
- The `'open'` handler emits `JSON.stringify({ replay_from: <seq> })`
  via `ws.send()` synchronously after `status = 'open'`, gated on
  `lastSeenSeq > 0`. Reconnect path runs through `reopen()` →
  `attachListeners()` so the first-frame ordering invariant holds.
- `close()` was left untouched — drops the record, which discards
  the cursor; a subsequent `subscribe()` opens with `lastSeenSeq = 0`
  and sends no frame.

Tests: `npm test -- --run src/lib/ws.test.ts` → **33/33 passing**.
The new suite covers fresh-subscribe (no frame), reconnect-after-
events (`{"replay_from":3}`), forward-only cursor, malformed `seq`
handling, explicit-close cursor invalidation, and chained reconnects
(advancing through two backoffs).

All six acceptance criteria from the substrate spec are demonstrably
satisfied by the test suite.

**Findings**

- Blockers: 0
- Important: 0
- Nits: 0

No parked items.
