---
id: story-fix-cli-playground-share-url
kind: story
stage: done
tags: [bug, cli, plugin]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# CLI prints invalid playground share URL

## Brief

`jamsesh jam new --playground` prints `Share URL:
https://<portal>/playground/<session-id>` (see
`cmd/jamsesh/sessioncmd/new.go:502`). That path doesn't match any SPA
route — `frontend/src/lib/router.svelte.ts` only registers
`/playground` (the landing), `/playground/s/:id/join` (the joiner
picker), and `/playground/s/:id/ended` (the tombstone). Visitors hitting
the printed URL fall through to the catch-all NotFound view, surfacing
as a "404".

Originally surfaced as `bug-playground-share-url-spa-404` during a
`/jamsesh:jam` test session. Initially scoped as a missing SPA route;
re-investigation during `feature-portal-visitor-entry-pages` design
showed the actual gap is the CLI emitting the wrong URL.

## Fix

In `cmd/jamsesh/sessioncmd/new.go:502`, change:

```go
shareURL := strings.TrimRight(baseURL, "/") + "/playground/" + resp.Session.Id
```

to:

```go
shareURL := strings.TrimRight(baseURL, "/") + "/playground/s/" + resp.Session.Id + "/join"
```

The `/playground/s/<id>/join` route is the canonical share surface per
`docs/UX.md` "Flow: joining a playground" — visitors land on the
`JoinerPicker.svelte` screen which prompts for a nickname and joins
them as a new playground participant.

## Acceptance

- Running `jamsesh jam new --playground` prints a `Share URL` that
  resolves to the `JoinerPicker.svelte` view in the SPA (visible
  rendered, not a NotFound).
- Manual verification: open the printed URL in a browser; the
  nickname picker renders.
- Unit test in `cmd/jamsesh/sessioncmd/new_test.go` (amend or add)
  asserts the printed URL format includes the `/s/<id>/join` segments.
- No change to the API call shape or the local state written — only
  the human-facing print line.

## Implementation notes

One-line change in `cmd/jamsesh/sessioncmd/new.go:512` (the line shifted
from 502 after recent edits). New test `TestPlaygroundAction_shareURLShape`
in `cmd/jamsesh/sessioncmd/new_test.go` asserts the printed URL contains
`/playground/s/<id>/join` and the old bare form is absent.

## Review (2026-05-24)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: One-line fix; the test is sharp (asserts the new path plus
anti-regression on the old). Brings code into alignment with `docs/UX.md`
"Flow: joining a playground" which already specified `/playground/s/{id}/join`
as the canonical share surface — no doc roll-forward needed.
