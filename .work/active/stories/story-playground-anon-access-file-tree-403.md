---
id: story-playground-anon-access-file-tree-403
kind: story
stage: done
tags: [playground, portal, auth, bug]
parent: feature-playground-anon-session-access
depends_on: []
release_binding: null
gate_origin: null
created: 2026-06-01
updated: 2026-06-01
---

# Anonymous playground file tree renders nothing (org-gated endpoints 403)

## Brief

An anonymous playground participant sees an empty file panel even after a
successful push. The data is present — the bare repo has
`refs/heads/jam/<id>/{base,draft}` at the pushed commit and the session row has
`base_sha` set — but the endpoints that populate the file UI reject the
anonymous bearer with `403 "not a member of this org"`.

Root cause: anon participants are `session_members` but never `org_members` of
`org_playground` (playground join calls `AddSessionMember` only —
`internal/portal/playground/handler.go:188,322`), while the org-scoped session
handlers check `GetOrgMember` first:

- `ListSessionRefs` — `internal/portal/sessions/state.go:109`
- `ListSessionFiles` — `internal/portal/sessions/files.go:46`
- also `refmodes.go:43`, `listing.go:38`, `invites.go:45`, `state.go:236`
  (digest)

Observed live 2026-06-01 (session `01KT0M1JPAMMSEXAQQBSTZFD7D`, v0.5.0): on the
canonical `/playground/s/<id>/resume` URL the playground session GET returns 200
but the follow-up `/api/orgs/org_playground/.../refs` returns 403.

Fix direction (org_members vs. per-endpoint session-membership fallback) is the
feature-level strategic decision — see the parent feature. This story owns the
server-side authorization change and its tests, whichever direction is chosen.

## Acceptance criteria

- An anonymous playground participant can list refs and files for their own
  session; the file panel renders the pushed tree.
- The fix does not widen access for non-members or across orgs (a non-member
  anonymous account still cannot read another session's refs/files).
- Regression test covers the playground anon path through the chosen
  authorization point.

## Design

Use session membership, not org membership, for concrete session-resource
endpoints. Anonymous playground accounts remain absent from `org_members`; the
server should authorize their own session resources because they are
`session_members`.

### Backend units

**Files**:
- `internal/portal/sessions/state.go`
- `internal/portal/sessions/files.go`
- `internal/portal/sessions/refmodes.go`
- `internal/portal/sessions/handler.go`

Replace leading `GetOrgMember` gates on concrete session-resource handlers with
the established auth helper:

```go
_, _, fail, ok := handlerauth.RequireSessionMember(ctx, h.store, orgID, sessionID)
if !ok {
    if fail.Err != nil {
        return nil, deperr.WrapDBIfTransient(fmt.Errorf("sessions: <op>: session member: %w", fail.Err))
    }
    return <operation>Fail(fail), nil
}
```

Target handlers:
- `GetSession`
- `ListSessionRefs`
- `GetSessionDigest`
- `GetSessionFile`
- `UpsertRefMode`

Keep `ListSessions` and session-invite creation org-member gated because those
are org inventory/invite surfaces, not playground session resources.

Add operation-specific auth-fail wrappers where needed, following the
`authfail-three-branch-guard` pattern:

```go
func listSessionRefsFail(f handlerauth.AuthFail) openapi.ListSessionRefsResponseObject {
    if f.Status == 401 {
        return openapi.ListSessionRefs401JSONResponse{UnauthorizedJSONResponse: f.Unauthorized}
    }
    return openapi.ListSessionRefs403JSONResponse{ForbiddenJSONResponse: f.Forbidden}
}
```

### Frontend unit

**File**: `frontend/src/lib/components/ArtifactPane.svelte`

Convert the manual `fetch()` to the shared typed client so the existing bearer
middleware can attach the active playground bearer:

```ts
const { data, error } = await client.GET(
  '/api/orgs/{orgID}/sessions/{sessionID}/files',
  {
    params: {
      path: { orgID: orgId, sessionID },
      query: { commit: selectedSha, path: selectedPath },
    },
    signal: controller.signal,
  },
);
```

Preserve the current abort-on-change behavior and the existing binary,
too-large, not-found, and generic error UI states.

## Tests

- Add per-dialect backend regression tests in
  `internal/portal/sessions/handler_test.go` or `files_test.go`:
  - seed `org_playground`, two sessions, an anonymous account bearer for
    session A, and a `session_members` row only for session A;
  - assert session A refs/digest pass without `org_members`;
  - assert session B refs/files fail with 403 for the session A bearer.
- Extend `frontend/src/lib/components/ArtifactPane` coverage or shell-level
  tests to prove file fetches go through `client.GET` and therefore inherit
  playground bearer selection.

## Implementation Notes

- Replaced the org-member gate on concrete session-resource handlers
  (`GetSession`, `ListSessionRefs`, `GetSessionDigest`, `GetSessionFile`, and
  `UpsertRefMode`) with `handlerauth.RequireSessionMember`; org inventory and
  invite surfaces remain org-member gated.
- Added strict-server auth-fail wrappers for the newly shared session-member
  guards.
- Added a per-dialect regression that seeds `org_playground`, two sessions, and
  an anonymous session member with no `org_members` row; own session refs/digest
  pass and cross-session refs/files return 403.
- Switched `ArtifactPane.svelte` from manual `fetch` + `auth.token` to the
  shared typed `client.GET`, preserving abort/stale-response behavior.

## Verification

- `go test ./internal/portal/sessions`
- `npm test -- --run src/lib/auth.test.ts src/lib/api/client.test.ts src/lib/components/ArtifactPane.test.ts src/App.test.ts src/lib/screens/SessionViewShell.test.ts src/lib/ws.test.ts`
- `npm run check` (0 errors, 1 pre-existing Svelte warning in `ModeSwitchDialog.svelte`)

## Review (2026-06-01)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Fast-lane story review. Implementation notes and green verification
are present; no lens walk run for story lane.
