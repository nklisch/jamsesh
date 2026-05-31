---
id: cli-resolvesession-env-var-mismatch
kind: story
stage: backlog
tags: [plugin, bug]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
---

# `ResolveSession` reads `CC_SESSION_ID` but instance_id is written from `CLAUDE_SESSION_ID`

Discovered during design of `epic-cli-browser-session-resume-cli-handoff`
(Codex xhigh advisory). Pre-existing; NOT introduced by the resume feature.

`cmd/jamsesh/sessioncmd/session.go` `ResolveSession()` matches the current CC
instance by reading `os.Getenv("CC_SESSION_ID")` and comparing it against each
session's `instance_id` sidecar (session.go:32). But that sidecar is WRITTEN
from `os.Getenv("CLAUDE_SESSION_ID")` (`join.go:264`), and
`state.CurrentSessionID()` (state.go:259) also keys off `CLAUDE_SESSION_ID`.

So `ResolveSession`'s CC-instance match can never succeed (it compares
`CC_SESSION_ID` to a value written from `CLAUDE_SESSION_ID`), silently falling
through to the "first session dir" branch. With a single session that's benign;
with multiple sessions per repo it resolves the WRONG session. `finalize`
(which uses `ResolveSession`) is affected, and bare `jamsesh resume` (new) would
be too.

Fix direction: pick ONE env var as the CC-instance identifier and use it
consistently across `ResolveSession`, `state.CurrentSessionID`, and the
`instance_id` write in `join.go`/`new.go`. Likely `CLAUDE_SESSION_ID` is the
intended one (it's what's written). Add a test asserting ResolveSession matches
a session whose instance_id == the env value, and errors/multi-session-guards
when unmapped.

Mitigation in the meantime: `epic-cli-browser-session-resume-cli-handoff`'s
`jamsesh resume` story uses the write-consistent resolver
(`state.CurrentSessionID`, `CLAUDE_SESSION_ID`) for bare resume rather than
`ResolveSession`, and references this item.
