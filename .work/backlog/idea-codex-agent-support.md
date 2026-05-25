---
id: idea-codex-agent-support
created: 2026-05-24
tags: []
---

Investigate OpenAI Codex (the CLI coding agent) as a second supported
agent runtime alongside Claude Code. Believe Codex now exposes a
lifecycle-hook surface — verify whether it covers the events jamsesh's
plugin depends on (SessionStart for context injection, UserPromptSubmit
for digest emission, PreToolUse for the `git push` deny, PostToolUse
for auto-push, Stop for auto-commit, SessionEnd cleanup). If the hook
surface is compatible, work out what a `plugins/jamsesh-codex/` would
look like and whether the underlying `jamsesh` binary can be shared
between both plugins. Expands jamsesh's reach beyond CC-only without
fragmenting the portal protocol.
