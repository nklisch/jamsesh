---
id: idea-data-dir-migration-helper
created: 2026-05-24
tags: [migration, release-notes]
---

The `story-data-dir-env-rename` refactor cuts over strictly from
`CLAUDE_PLUGIN_DATA` → `JAMSESH_DATA_DIR` (defaulting to
`${XDG_DATA_HOME:-$HOME/.local/share}/jamsesh`) with no back-compat
shim. Users whose CC plugin runtime previously set
`CLAUDE_PLUGIN_DATA` to a CC-managed directory (e.g.
`~/.local/share/claude/plugins/<id>/data/`) will find their state
(OAuth tokens, refresh_token, per-session bearers, instance_id
bindings, local session refs) orphaned at the old location post-upgrade
— the new binary writes to the XDG default and never looks at the old
path. They'll need to either re-authenticate and re-bind sessions, OR
manually `mv` the old directory contents to the new location. The
release notes for the version that ships this rename must call this
out explicitly, and we should consider shipping a one-time auto-migrate
helper (detect the old CC-managed path on first run after upgrade,
prompt the user, `mv` on confirmation) to reduce the upgrade burden.
