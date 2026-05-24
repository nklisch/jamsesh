---
id: idea-playground-scope-normalization-bug
created: 2026-05-24
tags: [bug, playground, cli]
---

In `cmd/jamsesh/sessioncmd/new.go` line 390, `newPlaygroundAction` skips `normalizeScope` when the scope value is `"**"` (the flag default), sending a raw string to the portal instead of the required JSON array `["**"]`. The portal rejects this with a 400 `session.invalid_writable_scope` error. Fix: remove the `req.Scope != "**"` guard so the default value is normalized the same way any other non-empty scope is — the condition should simply be `if req.Scope != ""`.
