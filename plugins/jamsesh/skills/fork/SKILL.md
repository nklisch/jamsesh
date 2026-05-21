---
name: fork
description: Fork from a commit (yours or a peer's) to create a new branch ref in the session
argument-hint: "<commit-sha> [--as <branch>] [--mode sync|isolated]"
---

# Fork from a commit

This is a jamsesh ref-management operation. The `jamsesh` skill (which
auto-loads with this plugin) covers the ref-namespace model
(`jam/<session>/<user>/<branch>`) and the two modes (sync, isolated) —
re-read it if you're unsure what mode the new ref should be in.

Run:

```bash
bin/jamsesh fork $ARGUMENTS
```

Surface the result. Print errors with their exit codes intact.

> The CLI and the `fork` MCP tool do the same thing on the server side.
> Prefer the MCP tool when you're already in a tool-use flow (see
> `jamsesh` skill section 7).
