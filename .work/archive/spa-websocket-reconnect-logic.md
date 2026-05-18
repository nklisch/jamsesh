---
id: spa-websocket-reconnect-logic
kind: feature
stage: done
tags: [ui]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# SPA WebSocket has no reconnect logic

## Finding

Discovered during e2e implementation of
`epic-e2e-tests-failure-mode-spa-error-states`. The Svelte SPA's
WebSocket layer at `frontend/src/lib/ws.svelte.ts` has no reconnect
behavior. The `close` event handler removes the socket from the map
without retry; users see no UI indicator when the connection drops.

## Why it matters

The portal's WebSocket gateway is the SPA's live-update channel
(commit.arrived, comment.added, merge.succeeded, conflict.detected,
etc.). When network conditions cause a drop:
- The user's session view silently stops updating
- No "reconnecting…" indicator surfaces
- The user might think the app is frozen or "done"

For a collaboration tool where peer activity is the headline feature,
silent WS death is a real UX failure.

## Design

### Reconnect loop (in-module)

`ws.svelte.ts` keeps the existing per-`sessionId` socket map; each
entry grows from a bare `WebSocket` to a small **per-session
connection record** owning:

- the current `WebSocket | null`
- `lastSeenSeq: number` — bumped whenever an envelope with `seq > lastSeenSeq` is delivered to a handler
- `status: 'connecting' | 'open' | 'reconnecting'` — exposed via the rune store
- `attempt: number` — current backoff exponent (reset to 0 on `open`)
- `closedByUs: boolean` — set by the exported `close(sessionId)` so the
  reconnect loop knows to stop

On `WebSocket.onclose`, if `closedByUs` is false AND the close code
indicates an unexpected closure (see "Close code policy" below), the
module:

1. Sets `status = 'reconnecting'`.
2. Computes `delay = min(BASE * MULT^attempt, CAP)` with a ±25% jitter.
3. `setTimeout(reopen, delay)`; on the timer firing, `attempt++` and
   reopen the socket. The new socket's `replay_from` first frame is
   set to `lastSeenSeq` (when > 0).
4. On successful `open`, reset `attempt = 0`, `status = 'open'`.

Constants: `BASE_MS = 1000`, `CAP_MS = 30000`, `MULT = 1.6`,
`JITTER = 0.25`. Sequence: 1.0s, 1.6s, 2.56s, 4.10s, 6.55s, 10.49s,
16.78s, 26.84s, 30.0s, 30.0s … (jittered).

The replay-from frame is sent **before** any other writes after
`open`, using the same JSON shape the portal expects per
`internal/portal/wsgateway/gateway.go:213`:
`{"replay_from": <lastSeenSeq>}`. The portal then streams events with
`seq > lastSeenSeq` before transitioning to live; the existing
message-handler dispatch in `ws.svelte.ts` routes them just like live
events. Idempotency of handlers is the consumers' responsibility (the
existing handlers append-and-dedupe by `seq` in
`ActivityFeed.svelte:146`).

### Close-code policy

`coder/websocket` writes `1006 Abnormal Closure` for network drops and
`1011 Internal Error` / `1008 Policy Violation` for server-side
forced closes. The portal currently issues `1008` only for
slow-consumer kicks (`gateway.go:127`). Reconnect is appropriate for:

- `1001` Going Away — server shutdown; reconnect picks up the next
  instance.
- `1006` Abnormal Closure — the common network-drop case (no close
  frame received).
- `1011` Internal Error — transient server bug; reconnect is harmless.
- `1012`–`1014` Service Restart / Try Again Later / Bad Gateway.
- **No code at all** — when the underlying transport tears down
  before any close frame arrives (browser fires close with code 1006).

Reconnect is **suppressed** for:

- `1000` Normal Closure — the server (or our own `close()` call) shut
  down cleanly; nothing to reconnect to.
- `1008` Policy Violation — the portal kicked us as a slow consumer;
  immediate reconnect would re-fill the buffer and re-trigger the
  kick. Surface a one-shot status and stop.
- `1003` Unsupported / `1007` Invalid Frame — programmer error;
  reconnecting can't help.
- `4401` / any `4xxx` application-level auth failure — the token is
  bad; the auth layer needs to act, not the WS layer. (The portal
  currently rejects bad tokens at HTTP-upgrade time with a 401, not
  via a close code, but we reserve `4401` for the future.)

The close-code list is encoded as a small `function shouldReconnect(code)`
predicate so the test suite can drive each branch.

### lastSeenSeq cursor

The cursor is **per session**, stored on the connection record (a
plain JS field — not a rune — because nothing reactive reads it). It
advances inside the existing `'message'` handler whenever the parsed
envelope's `seq` is a finite number greater than the current cursor.
Envelopes without `seq` (none today, but future-proof) don't move it.

When `close(sessionId)` is called explicitly (the SPA navigates away
from the session view, the user signs out, etc.), the entire record
is dropped — including the cursor. The next subscribe starts fresh
with `replay_from = 0` (i.e. no replay frame sent). This is the
intentional "fresh view = fresh stream" semantics; if a user wants to
catch up on missed history they refresh the session view, which loads
the persisted activity via REST before subscribing live.

### Status UI

The reconnect status is surfaced via a tiny `wsStatus` rune store
exported from `ws.svelte.ts`:

```ts
export const wsStatus: { for(sessionId: string): WsStatus };
```

Where `WsStatus = 'connecting' | 'open' | 'reconnecting'`. Consumers
that want to render a banner call `wsStatus.for(sessionId)` inside a
`$derived` and react when it changes to `'reconnecting'`. A small
**status banner component** at
`frontend/src/lib/components/WsStatusBanner.svelte` renders the
indicator: a single `role="status"` element with text "Reconnecting…"
plus a subtle accent pulse, mounted from `SessionViewShell.svelte`
just under the session header (above the tree/artifact grid). It is
absent (not just `visibility: hidden`) when the status is `'open'` so
screen readers don't announce nothing. `aria-live="polite"` ensures
non-intrusive announcement when the banner appears.

Visual: warning-muted background, warning text color, mono font for
the dot animation, sans for the label. All tokens are pulled from
`.mockups/design-system/tokens.css` (no new tokens). No separate
mockup is filed for this — see the "Mockups" section below.

### Go e2e helper

`tests/e2e/fixtures/wsclient/wsclient.go` gains:

```go
// ConnectFromSeq behaves like Connect but additionally writes a
// {"replay_from": <seq>} text frame as the first message after the
// WebSocket handshake, so the gateway replays missed events with
// seq > replaySeq before transitioning to live mode.
func ConnectFromSeq(ctx context.Context, t *testing.T, portalURL, sessionID, bearer string, replaySeq int64) *Client
```

Internally it shares the same dial + readLoop with `Connect`; the only
difference is a single `wsjson.Write` on the connection before the
read loop starts. This is the helper the existing Go-level integration
tests (`internal/portal/wsgateway/gateway_test.go`) already prove
out — we're surfacing it in the e2e fixtures layer.

## Design decisions

### D1 — Exponential backoff: 1.0s base, 30s cap, 1.6× multiplier, ±25% jitter

Suggested by the feature brief (1s/2s/4s/8s → 30s). Adopted 1.6×
instead of 2× to soften the climb (peers fan in less aggressively
when a portal restart kicks N tabs simultaneously). Jitter prevents
thundering-herd reconnects after a portal restart.

### D2 — Cursor lives on the connection record, not a rune

`lastSeenSeq` is read by exactly one place — the reconnect-time
replay-frame builder — and written by exactly one place — the message
handler. Nothing reactive consumes it. A plain field on the
connection record matches the data flow; a rune would suggest
subscribers can read it, which would invite drift bugs (every
`subscribe()` handler already gets the seq inside the envelope).

### D3 — `wsStatus` IS a rune store

Status, by contrast, is consumed reactively by
`WsStatusBanner.svelte`. It follows the same wrapper-object pattern
as `auth.svelte.ts:21` (`get` accessors closing over a private
`$state`) so we get reactivity without exporting the rune
expression directly.

### D4 — Cursor invalidation on `close(sessionId)`

Calling `close()` drops the cursor along with the connection record.
Rationale: a closed-then-resubscribed flow is semantically "I left
the view and came back", not "I lost the connection briefly". The
session view component owns hydration via REST anyway, so a fresh
live stream is the right default. The reconnect loop, by contrast,
KEEPS the cursor across socket replacements because the user
conceptually never left.

### D5 — Single status banner per session, mounted at the shell

The banner is mounted once in `SessionViewShell.svelte`, not per
WebSocket-using component. A session has exactly one live socket;
showing a banner per component (ActivityFeed, TreeDag, CommentsTab)
would stack three identical banners. The shell is the only place
that knows the session is currently being viewed.

### D6 — Skip mockup for status banner

Per project CLAUDE.md UI/UX convention, novel UI surfaces get mocked
first. The banner is a single `role="status"` text element using
existing design tokens (`--color-warning`, `--color-warning-muted`,
`--font-sans`) and follows the same compact-strip pattern as the
`.bottom-latest`/`.live-dot` row already in
`SessionViewShell.svelte:260-263`. No new visual language is
introduced. This call is registered explicitly per convention so
reviewers can flag if they disagree.

### D7 — Close codes that suppress reconnect

`1000` Normal Closure (we asked for it), `1008` Policy Violation
(slow-consumer kick — re-connecting would just retrigger it),
`1003`/`1007` (programmer error — not transient), and the reserved
`4xxx` app range (auth failures the auth layer should handle). All
other codes — including the bare-1006 transport-loss case — trigger
reconnect. The predicate is a small pure function so each branch
can be unit-tested without spinning up a real socket.

### D8 — Replay frame sent before consumers can write

The portal accepts `replay_from` only as the first text frame after
upgrade (gateway.go:215). The reconnect loop sends it inside the
`'open'` listener, synchronously, before resolving any deferred
sends. Nothing else writes to the socket today (the client doesn't
issue commands over WS), so there's no contention; we add a
defensive comment in the module head explaining the invariant.

## Mockups

This feature adds a single `role="status"` text banner that reuses the
existing design system (`--color-warning`, `--color-warning-muted`,
mono+sans). Per D6, no separate mockup is filed; the visual closely
mirrors the existing `bottom-latest` strip in
`SessionViewShell.svelte`.

## Child stories

Four child stories, decomposed by file boundary to minimize cross-PR
conflicts. The wsclient helper is independent of the SPA work and can
ship in parallel.

1. **`spa-websocket-reconnect-logic-backoff`** — Reconnect loop +
   close-code predicate + `wsStatus` rune store in `ws.svelte.ts`. The
   foundation for the other SPA stories. No replay yet; reconnect
   simply re-subscribes with `replay_from = 0`.

2. **`spa-websocket-reconnect-logic-replay-from`** — `lastSeenSeq`
   cursor tracking and `replay_from` first-frame emission on
   reconnect. Depends on `-backoff` for the reconnect path.

3. **`spa-websocket-reconnect-logic-status-ui`** —
   `WsStatusBanner.svelte` component + mount in
   `SessionViewShell.svelte` + un-skip the Playwright test in
   `tests/e2e/playwright/error-states.spec.ts`. Depends on `-backoff`
   for `wsStatus`.

4. **`spa-websocket-reconnect-logic-wsclient-helper`** —
   `ConnectFromSeq` in `tests/e2e/fixtures/wsclient/wsclient.go`. No
   SPA deps; ships independently.

## Suggested implementation (rolled into Design)

Superseded by the `## Design` section above. Kept here as a
historical pointer for the reader scanning the original finding.

## Acceptance criteria

- [ ] `ws.svelte.ts` retries with exponential backoff on unexpected
      close
- [ ] On reconnect, the client passes `replay_from: <last-seen-seq>`
      so missed events are delivered
- [ ] A visible UI indicator (text + role="status") shows the
      reconnecting state
- [ ] The Playwright test
      `network_loss_state_shows_reconnecting_indicator_in_session_view`
      in `tests/e2e/playwright/error-states.spec.ts` is un-skipped
      and passes
- [ ] `tests/e2e/fixtures/wsclient/wsclient.go` gets a
      `ConnectFromSeq` helper so the e2e Go layer can drive
      reconnect scenarios too

## Notes

The Playwright test is skipped with `test.skip` and a comment
pointing at this story. Re-enable when reconnect + status UI lands
(handled inside the `-status-ui` story).

## Review

**Verdict:** Approve.

Feature-level review of the full reconnect layer shipped across four
child stories (all at `stage: done` and individually approved). Each
acceptance criterion in the feature body is demonstrably satisfied:

- **Exponential backoff on unexpected close** — `ws.svelte.ts`
  implements the `BASE=1000ms`, `CAP=30 000ms`, `MULT=1.6`,
  `JITTER=±25%` envelope with a `shouldReconnect(code)` predicate
  matching D7 verbatim. Covered by 7 backoff tests + 3 predicate tests
  in `ws.test.ts`.
- **`replay_from` first frame on reconnect** — `ws.svelte.ts:174`
  emits `ws.send(JSON.stringify({ replay_from: rec.lastSeenSeq }))`
  gated on `lastSeenSeq > 0`. Cross-checked against the portal:
  `internal/portal/wsgateway/gateway.go:213-216` reads the frame via
  `wsjson.Read` into `{ ReplayFrom int64 \`json:"replay_from"\` }`,
  then `g.Log.ListSince(sessionID, res.seq, 1000)` streams events with
  `seq > N` before transitioning to live. Wire format and semantics
  match the SPA's emission exactly.
- **Visible reconnecting indicator (text + `role="status"`)** —
  `WsStatusBanner.svelte` renders `role="status"` /
  `aria-live="polite"` with text "Reconnecting…" only when
  `wsStatus.for(sessionId) === 'reconnecting'` (absent otherwise, per
  the accessibility constraint). Mounted exactly once between
  `.session-header` and the top grid in `SessionViewShell.svelte`,
  per D5.
- **Playwright network-loss test un-skipped** — `test.skip(…)` →
  `test(…)` in `tests/e2e/playwright/error-states.spec.ts:238`;
  asserts `page.getByRole("status", { name: /reconnecting/i })`.
- **`ConnectFromSeq` helper in Go fixtures** —
  `tests/e2e/fixtures/wsclient/wsclient.go:68` ships
  `ConnectFromSeq(ctx, t, portalURL, sessionID, bearer, replaySeq)`
  sharing a `dial` helper with `Connect`; writes one
  `{"replay_from":N}` frame for `N>0`, nothing for `N<=0`. Covered by
  4 in-process httptest fixtures (Docker-free) plus the existing
  portal-side coverage in
  `internal/portal/wsgateway/gateway_test.go::TestHandler_ReplayFromCursor`.

**Integration verification:**

- `cd frontend && npm test` → 35 files, **333/333 passing**.
- `cd frontend && npm run check` → 6 pre-existing errors only, all in
  `RefGroupList.test.ts` (unrelated `Set<unknown>` vs `Set<string>`);
  zero new errors or warnings introduced by this feature.
- `cd tests/e2e && go build ./fixtures/wsclient/...` → clean.

**Cross-feature contract:** SPA emit shape matches portal expectation
1:1. The portal accepts the frame only as the first text frame post-
upgrade (within a 2s grace, after which it falls through to live), and
the SPA emits it synchronously inside the `'open'` listener before
anything else writes — satisfying D8.

**Findings:**

- Blockers: 0
- Important: 0
- Nits: 0 (already addressed at story level — the documented dead
  `stubGateway.connectionsCh` field and stale "27 tests" comment in
  `-backoff` story body. Pure docs/test-fixture drift, not worth
  filing.)

No parked items. Advancing `review → done`.
