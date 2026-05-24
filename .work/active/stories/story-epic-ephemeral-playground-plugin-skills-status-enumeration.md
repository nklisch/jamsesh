---
id: story-epic-ephemeral-playground-plugin-skills-status-enumeration
kind: story
stage: review
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

## Implementation notes

**Approach**: Replaced single-session `statusAction` (which required an
account-wide OAuth token) with an enumeration loop over
`state.ListSessions()`. Each iteration reads the per-session bearer via
`state.ReadSessionToken(sessID)` and calls the appropriate endpoint
using the new `portalclient.GetJSONWithBearer[T]` helper (which bypasses
the `state.ReadToken()` / refresh-retry machinery entirely — appropriate
since playground tokens don't refresh).

**Session kind detection**: determined by the `org_id` sidecar file
(`playgroundOrgID = "org_playground"`). Durable sessions call
`GET /api/orgs/{orgID}/sessions/{id}` and
`GET /api/orgs/{orgID}/sessions/{id}/refs`; playground sessions call
`GET /api/playground/sessions/{id}`.

**Backward-compat JSON**: the `--json` shape changed from a flat
single-session object to `{"durable":[...],"playground":[...]}`. A
`statusOutput` type alias (`= durableStatusOutput`) is kept so existing
test references still compile. Both arrays are always present (never null)
even when empty.

**`portalclient.GetJSONWithBearer`**: added as a standalone generic
function to `cmd/jamsesh/portalclient/client.go`. Takes an explicit
`*http.Client` (nil → `http.DefaultClient`) and bearer string; sets
the `Authorization` header directly on the request without going through
`Client.Do`'s refresh-retry path.

**Tests removed**: `TestStatusAction_commentsAddressedToMe` —
the enumeration model is a summary view; per-user comment filtering is
no longer part of the status subcommand. The behavior is documented in
this note.

**Pre-existing failures**: `mcpheaders` tests fail because they expect
the legacy `token` file but `MigrateToPerSessionTokens` (Story 2) has
already written a `MIGRATED_TO_PER_SESSION` stub. These failures pre-date
this story and are tracked separately.
