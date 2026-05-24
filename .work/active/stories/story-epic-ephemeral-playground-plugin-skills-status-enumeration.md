---
id: story-epic-ephemeral-playground-plugin-skills-status-enumeration
kind: story
stage: implementing
tags: [plugin, playground]
parent: feature-epic-ephemeral-playground-plugin-skills
depends_on: [story-epic-ephemeral-playground-plugin-skills-bearer-storage]
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# `/jamsesh:status` enumeration under anon-mode

## Scope

Story 3 of the parent feature. Updates `cmd/jamsesh/sessioncmd/status.go`
to enumerate per-session tokens (from the wave-2 storage migration in
Story 2) rather than requiring an account-wide OAuth token. Status
output groups durable and playground sessions separately.

Full design in the parent feature body's "Story 3" section.

## Files delivered

- `cmd/jamsesh/sessioncmd/status.go` (modify)
- `cmd/jamsesh/sessioncmd/status_test.go` (extend)

## Acceptance criteria

See parent feature body's "Story 3 acceptance criteria" section.

## Notes

- Depends on Story 2 (bearer storage) — needs `state.ReadSessionToken`
  + `state.ListSessions` helpers.
- Status output JSON shape must be backward-compatible: existing fields
  for durable sessions stay; playground sessions get a separate top-level
  array. Don't break consumers that parse the existing JSON.
- The pre-launch reality means there are no existing consumers anyway,
  but design for forward compatibility — once status output is shipped
  in v0.4.0, it becomes a contract.
- Missing per-session token (e.g., manual deletion) is a warning, not
  a fatal error — skip the session and continue.
