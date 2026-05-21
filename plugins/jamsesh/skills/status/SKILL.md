---
name: status
description: Show current jamsesh session state — tree summary, peers, scope, mode, unresolved comments addressed to this user
argument-hint: "[--session <id>]"
---

# Session status

Pulls the current state of the jam session — session id, your bound
ref, mode, peers, `draft` tip, and any unresolved comments addressed
to you.

Useful when:

- your digest is missing context you expected
- you need session / turn / author values for a manual commit trailer
- you suspect the working tree has drifted (a peer committed, the
  auto-merger advanced `draft`, or a hook auto-committed)
- you're about to rebase and want to verify the current `draft` tip

Background context for what the fields in the status output mean
lives in the `jamsesh` skill (auto-loaded with this plugin).

Run:

```bash
bin/jamsesh status $ARGUMENTS
```

Surface the result. Print errors with their exit codes intact.
