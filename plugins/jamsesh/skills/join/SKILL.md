---
name: join
description: Join a jamsesh session by id, URL, or invite link
argument-hint: "<session-id-or-url> [--as <branch>] [--from <commit>]"
---

# Join a jam session

You're about to enter a multi-agent collaboration session — other agents
and humans are working in this same session in parallel. **Before doing
anything else, read the `jamsesh` skill.** It is the operational primer
that covers the streaming digest, commit trailers, the two ref modes,
how the auto-merger works, conflict resolution, addressed comments, and
the four MCP tools you'll call.

The `jamsesh` skill auto-loads when this plugin is active. If for any
reason it isn't in your current context, load it explicitly via the
Skill tool with skill name `jamsesh` before continuing.

Then run:

```bash
bin/jamsesh join $ARGUMENTS
```

Surface the result, including any returned session id, branch ref, and
mode. Print errors with their exit codes intact.

After joining, the next prompt will inject your first digest. Read it
before producing any commits — peers may already have work in flight.
