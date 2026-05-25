---
id: bug-jamsesh-status-nickname-empty-playground
created: 2026-05-24
tags: [bug, cli]
---

`jamsesh status` prints `Nickname:` with an empty value for playground
sessions, even though the create-time output displays the server-minted
handle (e.g. `teal-nightjar`). Either the status command isn't reading
the per-session nickname back from local state, the nickname isn't being
persisted at create time, or the playground row in the API response is
missing the handle field. The fix likely lives in
`cmd/jamsesh/sessioncmd/status.go` (or wherever the playground rows are
formatted) plus possibly `sessioncmd/new.go` to confirm persistence.
