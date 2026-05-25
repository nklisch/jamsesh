---
id: gate-docs-pattern-line-anchors-drift-rerun
kind: story
stage: implementing
tags: [documentation]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: docs
created: 2026-05-24
updated: 2026-05-24
---

# Pattern-skill line anchors stale across 5 patterns (re-run drift)

## Drift category
pattern-skill-staleness

## Location
Five pattern skills under `.claude/skills/patterns/` carry line anchors
that point to the wrong line after the bundle's refactors and recent
doc-roll commits. Each anchor still resolves to a real file, but the
`file:line` cursor lands on a different statement than the snippet shown.

**Anchors to fix:**

1. `.claude/skills/patterns/spec-driven-event-types.md`
   - Example 1: `internal/portal/sessions/handler.go:155` → actual call to
     `events.Emit(..., "session.created", ...)` is at line **169**.
   - Example 2: `internal/portal/automerger/worker.go:352` → actual
     `Log.Emit(..., "auto-merger.backpressure", ...)` is at line **358**.

2. `.claude/skills/patterns/playground-activity-reset.md`
   - Example 1: `internal/portal/comments/service.go:214` → actual
     `ResetSessionIdleTimer(...)` call is at line **226** (and the trailing
     summary line also lists `:214` — both must roll forward).
   - Example 2: `internal/portal/sessions/handler.go:323` → actual is line **339**.
   - Example 3: `internal/portal/githttp/receive_pack.go:336` → actual is line **338**.

3. `.claude/skills/patterns/ticker-sweep-loop.md`
   - Example 1: `internal/portal/playground/worker.go:62` → actual
     `time.NewTicker(w.Interval)` is at line **72**.

4. `.claude/skills/patterns/strict-server-partial-handler-shim.md`
   - Example 1: `internal/portal/playground/handler_test.go:89` → actual
     `type playgroundOnlyStrict struct` is at line **75**.
   - Example 3: `internal/portal/accounts/handlers_test.go:84` — anchor
     resolves to the doc-comment line, which IS what the pattern cites
     (OK as-is, but verify intent).
   - Summary list cross-references (lines 74-79 of the pattern):
     - `auth/magic_link_test.go:111` → first `magicLinkOnlyStrict` reference
       at line **94**.
     - `auth/oauth_test.go:51` → actual `oauthOnlyStrict` doc comment at line **50**.
     - `comments/service_test.go:42` → actual at line **43**.
     - `tokens/handlers_test.go:29` → actual `tokensOnlyHandler` doc at line **26**.
     - `wsgateway/ticket_handler_test.go:34` → actual at line **32**.

5. `.claude/skills/patterns/openapi-fetch-result-branch.md`
   - Example 2: `frontend/src/lib/screens/InviteAccept.svelte:80` → actual
     `client.POST('.../invites/{inviteID}/accept', ...)` is at line **83**.
   - Example 3: `frontend/src/lib/screens/OrgSettings.svelte:32` → actual
     `Promise.all([client.GET(...), client.GET(...)])` is at line **22**.

## Current doc text
Each pattern file pins the relevant snippet with a `**File**:
`path/to/file:line`` header followed by the code example. The line numbers
above are stale by 2-14 lines for each cited anchor.

## Reality
Recent refactors (god-component split, per-package clock compliance,
adapter dedup) and intervening doc commits shifted line numbers in the
target source files without updating the pattern anchors. The pattern
text + snippet are still correct; only the line numbers drift.

## Required edit
For each pattern file listed above, update the cited `file:line` anchor to
the current line number. Where a pattern shows the WHOLE snippet (with
surrounding code), verify the snippet still matches the source — if the
source has changed shape, update the snippet too. Apply rolling-foundation:
just replace the stale line numbers with current ones; no `(was N, now M)`
prose.

Note: the `openapi-fetch-middleware-client` pattern (already done in the
original gate-docs pass) intentionally pins anchors to symbols rather than
line numbers for files under active refactor. Consider extending that
"symbol-based anchor" approach to the five patterns above as a follow-up
refactor — but the immediate fix is just line-number roll-forward.
