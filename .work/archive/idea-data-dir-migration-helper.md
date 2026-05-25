---
id: idea-data-dir-migration-helper
kind: story
stage: done
tags: [plugin, migration, release-notes]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-25
---

## Implementation notes

- `cmd/jamsesh/state/migrate.go`: added
  `DetectCCManagedLegacyData(logger Logger) (warned bool)` and the helper
  `hasMigratableState(dir string) (bool, error)`.
- Behaviour: read `CLAUDE_PLUGIN_DATA` env. If set, the path is different
  from `JAMSESH_DATA_DIR`, the old path has at least one of
  `token`/`refresh_token`/non-empty `sessions/`, AND the new path is
  empty — emit a structured `Warn` naming both paths and a copy-paste
  `mv` command. Returns `true` so callers can gate interactive notices.
- **Deviation from "consider auto-move"**: the implementation does NOT
  auto-move directories. Silent moves can surprise self-hosters who
  deliberately manage shared directories; the one-time mv cost is small
  and the warning gives a copy-paste command. Documented here as the
  conscious choice — easier to relax later than to walk back a silent
  move.
- `cmd/jamsesh/main.go`: wires `state.DetectCCManagedLegacyData(stderrLogger{})`
  right after the per-session token migration on every invocation. Idempotent.
- Six new tests in `migrate_test.go` cover: no env var, env set + old empty,
  old token only, old sessions only, both paths populated (already
  migrated), and env points at same dir as JAMSESH_DATA_DIR.

Verified: `go test ./cmd/jamsesh/state/... -count 1` passes.

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

## Review (2026-05-25)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Best-effort detector with idempotent guards, structured logging, and a copy-paste `mv` command. The "no auto-move" deviation is appropriately documented — silent moves on shared/operator-managed dirs would be worse than the one-time UX cost. Six tests cover the relevant matrix (no env, empty old, token-only, sessions-only, both populated, same-path). Release-notes call-out is the responsibility of the release-deploy stride.
