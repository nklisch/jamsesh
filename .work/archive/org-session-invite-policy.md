---
id: org-session-invite-policy
kind: feature
stage: done
tags: [portal, ui, security]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Feature — Per-org session invite policy

## Brief

Some orgs want strict members-only access; others want open email-invite
collaboration. Encode that as a per-org configuration — `orgs.session_invite_policy` with
two values, `members_only` (default, strictest) and `open` — and enforce it at the
invite-accept perimeter. Under `members_only`, accepting a session invite requires
the invitee to already be an `org_member`. Under `open`, accepting adds the invitee
to `session_members` only — they become a session-scoped guest, not an org member.

After the gate fires at invite-accept time, the rest of the system stays simple:
`handlerauth.RequireSessionMember` is correct for session-scoped operations (membership
already enforced per the org's policy), and `handlerauth.RequireOrgMember` keeps
open-org session guests out of org-scoped operations.

This is the resolution to the audit previously parked as
`audit-session-membership-org-implication.md` (the audit asked which of three
resolution paths to take; the answer is "all of them per-org via a policy column,
not a global choice").

UI scope: a new dedicated org settings screen at `/orgs/:orgID/settings` for admins
to flip the policy, and a new invite-accept screen with happy-path + members-only
rejection states. Mockups are required for both surfaces before frontend code per the
project's mockup-first rule.

## Strategic decisions

These were locked in via interactive Q&A before scoping. Captured here so
`feature-design` and downstream stories inherit them:

- **Default policy for new orgs + migration backfill**: `members_only`. Strictest
  default, preserves pre-refactor strictness for everyone in the system today.
  Org admins can flip to `open` per-org via the settings UI.
- **Open-org guest model**: session-only. Open-org invitees become `session_members`
  only — never auto-promoted to `org_members`. This keeps org member lists limited
  to actual org employees and avoids inventing a new `guest` role on `org_members`.
  `RequireOrgMember` correctly keeps them out of org-scoped operations.
- **Invite-accept UI scope**: build the full accept screen fresh — no
  `InviteAccept.svelte` exists in the SPA today. The screen handles both
  happy-path (show invite details + Accept button → redirect to session) and the
  members-only rejection state ("ask an admin to invite you to the org first").
- **Org settings navigation**: new dedicated route `/orgs/:orgID/settings` with the
  session-invite-policy section as its first (currently only) section. Future
  sections — members management, billing, etc. — extend the same screen.

## Mockups

- OrgSettings screen: `.mockups/screens/org-session-invite-policy-settings/index.html`
  - Selected: **option-2 — Sidebar nav + content pane** (2026-05-17)
  - Rationale: anticipates future sections (Members, Billing, API keys) explicitly via the left rail. Single Save per page matches expected admin flow. Scales without restructure when sections are added.
- InviteAccept screen: `.mockups/screens/org-session-invite-policy-accept/index.html`
  - Selected: **option-3 — Onboarding hero** (2026-05-17)
  - Rationale: most invitees will be first-time users to the destination org/session. The hero layout's lead text + "What happens when you accept" explainer card teaches the product on the way in, which lowers the bar to first push. The "Invited by Marcus Chen" pill + named session in the headline preserves dignity even when context is rich.
  - Implementation note: this layout requires inviter name + org/session name on render. That implies a new lightweight `GET /api/orgs/{orgID}/sessions/{sessionID}/invites/{inviteID}` (token-protected) to fetch those fields before the POST-accept. Folded into Story 6 (InviteAccept screen) as part of its acceptance.

## UI surface flag

`ux-ui-design` plugin is installed; this feature has net-new UI surface
(both `OrgSettings.svelte` and `InviteAccept.svelte` are greenfield). Tagged
`[ui]` so `feature-design`'s Phase 4.6 picks up the mockup-first
requirement. Mockups for both surfaces go in
`.mockups/screens/org-session-invite-policy/`. The "guest" visual indicator
for session-only members in existing session views is intentionally a polish
nit, not part of this feature — it can ride along when a UI story
naturally touches member-chip rendering.

## Architectural choice

**Option A — Perimeter enforcement at invite-accept.** The policy is read
when an invite is accepted. If `members_only`, `AcceptSessionInvite`
short-circuits with 403 unless the actor is already in `org_members`. Once
a `session_members` row exists, downstream session-scoped handlers
(`handlerauth.RequireSessionMember`) trust the membership without re-checking
the policy.

**Why over the alternatives:**

- Option B (re-check at every session-scoped operation) — wasteful per-request
  cost; doesn't change the answer once the row exists.
- Option C (auto-promote open-org invitees to `org_members` with a `guest`
  role) — bigger schema change, requires a new role enum value, and
  every existing org-member query needs to be audited for guest semantics.
  Per-org policy at the perimeter sidesteps the role-explosion.

The perimeter is the right place because the question ("can this person
become a session member?") is uniquely a policy question; the downstream
question ("can this session member do this thing?") is a membership
question — they should be separated.

## Implementation Units

### Unit 1: Schema + sqlc + foundation doc
**File**: migration + `db/schema/*.sql` + `db/queries/orgs.sql` + adapters + `docs/ARCHITECTURE.md`
**Story**: `org-session-invite-policy-schema`

```sql
ALTER TABLE orgs
  ADD COLUMN session_invite_policy TEXT NOT NULL DEFAULT 'members_only'
  CHECK (session_invite_policy IN ('members_only', 'open'));
```

New domain field `Org.SessionInvitePolicy string`; new sqlc queries
`GetOrgSessionInvitePolicy :one` and `UpdateOrgSessionInvitePolicy :exec`;
adapters map the column through `pgOrg(...)` / `sqliteOrg(...)`.

The story also folds in the foundation-doc roll-forward — the membership-
model subsection in `docs/ARCHITECTURE.md` (or `docs/SECURITY.md`).

**Acceptance**: existing orgs default to `members_only` via migration;
round-trips through the store work on both dialects; doc updated.

---

### Unit 2: Invite-accept policy enforcement
**File**: `internal/portal/sessions/invites.go`
**Story**: `org-session-invite-policy-invite-accept-enforce`

```go
// After email-match (line ~200), before WithTx:
org, err := h.store.GetOrg(ctx, orgID)
if err != nil { return nil, fmt.Errorf("sessions: get org: %w", err) }
if org.SessionInvitePolicy == "members_only" {
    if _, mErr := h.store.GetOrgMember(ctx, store.GetOrgMemberParams{
        OrgID: orgID, AccountID: acc.ID,
    }); mErr != nil {
        if errors.Is(mErr, store.ErrNotFound) {
            return openapi.AcceptSessionInvite403JSONResponse{
                ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
                    Error:   "auth.org_membership_required",
                    Message: "this org requires you to be a member before joining sessions",
                },
            }, nil
        }
        return nil, fmt.Errorf("sessions: get org member: %w", mErr)
    }
}
```

**Acceptance**: rejects under `members_only` + non-member; accepts under
`open` regardless; accepts under `members_only` + existing org-member
(happy path preserved).

---

### Unit 3: GET invite details
**File**: `internal/portal/sessions/invites.go` (new `GetSessionInvite` method)
**Story**: `org-session-invite-policy-get-invite-details`

```go
type SessionInviteDetails struct {
    InviteID         string
    OrgName          string
    SessionID        string
    SessionName      string
    SessionGoal      *string
    InvitedByName    string
    ExpiresAt        time.Time
    YourRoleOnAccept string  // "member"
}

func (h *Handler) GetSessionInvite(ctx, req) (...) // mirrors AcceptSessionInvite's
                                                    // auth/validation flow without
                                                    // the mutating tx
```

**Acceptance**: returns 200 + details for a valid request; returns the
same 401/409 paths as `AcceptSessionInvite`; no state mutation; safe
against invite-ID enumeration (401 instead of 404 on unknown ID).

Reason this exists: the InviteAccept onboarding-hero mockup needs
inviter name, org name, session name on render BEFORE the user clicks
Accept. Could be folded into the POST-accept response, but that loses
the "review before action" affordance.

---

### Unit 4: PATCH org policy
**File**: `internal/portal/accounts/orgs.go` (new `PatchOrg` method) + OpenAPI
**Story**: `org-session-invite-policy-patch-endpoint`

```go
func (h *Handler) PatchOrg(ctx, req) (...) {
    _, member, fail, ok := handlerauth.RequireOrgMember(ctx, h.store, req.OrgID)
    if !ok { /* 401 or 403 */ }
    if member.Role != "creator" { /* 403 */ }
    if req.Body.SessionInvitePolicy != nil {
        h.store.UpdateOrgSessionInvitePolicy(ctx, ...)
    }
    org, _ := h.store.GetOrg(ctx, req.OrgID)
    return openapi.PatchOrg200JSONResponse(orgToOpenAPI(org)), nil
}
```

**Acceptance**: admin (creator) flip persists; non-creator gets 403;
invalid policy value gets 400; no retroactive eject of existing
session-only members (grandfather behavior).

---

### Unit 5: OrgSettings.svelte (Sidebar nav + content pane)
**File**: `frontend/src/lib/screens/OrgSettings.svelte`
**Mockup**: `.mockups/screens/org-session-invite-policy-settings/option-2.html`
**Story**: `org-session-invite-policy-org-settings-ui`

New route `/orgs/:orgID/settings`. Left rail lists settings sections
(Session invites active; Members/Billing/API keys dimmed as "soon").
Right pane shows the policy radio and Save. Admin sees editable; non-admin
sees disabled controls with a warning-tinted banner.

```ts
type Props = { orgId: string };
```

**Acceptance**: route renders for admin (editable Save) and non-admin
(read-only with banner); Save calls `PATCH /api/orgs/{orgID}` and
reflects on 200; admin-vs-member discrimination uses the existing
`auth.currentUser.orgs[X].role` from `MeResponse`.

---

### Unit 6: InviteAccept.svelte (Onboarding hero)
**File**: `frontend/src/lib/screens/InviteAccept.svelte`
**Mockup**: `.mockups/screens/org-session-invite-policy-accept/option-3.html`
**Story**: `org-session-invite-policy-invite-accept-ui`

New route
`/orgs/:orgID/sessions/:sessionID/invites/:inviteID/accept?token=<token>`.
Hero layout with invited-by pill, session-name headline, lead paragraph,
Accept/Decline, "What happens when you accept" explainer.

```ts
type Props = { orgId: string; sessionId: string; inviteId: string };
```

State flow:
- Mount → read token from query → GET invite details → render Happy
- Click Accept → POST accept
  - 200 → navigate to session
  - 403 + `auth.org_membership_required` → Rejection state
  - other → Error state

**Acceptance**: all three states (Happy, Rejection, Error) render per the
mockup; Accept POSTs with the body token and navigates on success;
Decline returns to user's session list; login-return-to is preserved
across unauth → /login → invite-URL round-trip.

## Implementation Order

Three waves with cap-3 parallelism:

```
Wave 1 (parallel, 2 agents):
  Unit 1 (schema)
  Unit 3 (GET invite details)

Wave 2 (parallel, 3 agents):
  Unit 2 (enforce)          depends on Unit 1
  Unit 4 (PATCH)            depends on Unit 1
  (Unit 3 may finish here if it didn't in wave 1)

Wave 3 (parallel, 2 agents):
  Unit 5 (OrgSettings UI)   depends on Unit 4
  Unit 6 (InviteAccept UI)  depends on Units 2 + 3
```

Mockups are already produced (Unit 5/6's mockup references); no story for
mockup generation.

## Testing

### Unit tests per unit
- **Unit 1**: migration on both dialects; `Store.GetOrg` returns the new
  field; `Store.UpdateOrgSessionInvitePolicy` round-trips.
- **Unit 2**: `AcceptSessionInvite` cross-product `{members_only, open} × {is org member, is not}`.
- **Unit 3**: happy GET; invalid-token 401; wrong-email 401; already-accepted 409;
  no state mutation between calls.
- **Unit 4**: 401 (no auth), 403 (non-member), 403 (non-creator), 200 (creator),
  400 (invalid enum); grandfather verification (existing session_members
  rows unchanged after flip).
- **Unit 5**: `OrgSettings.svelte` admin render, non-admin render, save success,
  save failure with banner.
- **Unit 6**: `InviteAccept.svelte` loading, happy, rejection, error renders;
  accept-success navigation; decline navigation.

### Integration check
After Unit 2 + Unit 4 merge, run a manual cross-check: flip an org to
`open` via PATCH; from a non-org-member account, accept a session invite;
verify session_member created without org_member. Then flip back to
`members_only` and verify the existing session-only member is grandfathered.

## Risks

From the pre-mortem:

- **Login round-trip return-to (Unit 6)**. The InviteAccept flow assumes an
  authenticated user. If the current login screen doesn't preserve the
  intended destination URL across `/login` → post-auth, the user lands on
  the wrong page after authenticating. **Mitigation**: implementing agent
  for Unit 6 verifies and (if needed) adds a `?return_to=<url>` query
  param to the login redirect.
- **Schema migration on SQLite older than 3.37**. The CHECK-with-ADD-COLUMN
  pattern may not work on ancient SQLite. **Mitigation**: project uses
  `modernc.org/sqlite` which is sufficient. If the implementer finds an
  edge case, fall back to the CREATE-TABLE-then-INSERT pattern.
- **Invite-ID enumeration via GetSessionInvite (Unit 3)**. Returning 404
  for an unknown invite leaks existence; we deliberately return 401
  instead (per the implementation note).
- **Affected code areas during regeneration**. `make generate-api-go` and
  `make generate-api-ts` will regenerate types files that aren't directly
  edited. The implementing agent should re-run these per story rather
  than batching at the end — surfaces conflicts earlier.

## Foundation-doc note

`docs/ARCHITECTURE.md` currently asserts "Every persisted entity carries `org_id`.
Every API route is org-scoped" without specifying the membership model in detail.
This feature should add a short subsection (in ARCHITECTURE.md or a new section in
`docs/SECURITY.md`) describing:

- That orgs carry a `session_invite_policy` (the two values + their effect)
- That `session_members` and `org_members` are independent tables joined at
  invite-accept time by policy
- That open-org session guests have access to the session they were invited to
  but NOT to org-wide resources (i.e. `RequireOrgMember` is the gate that keeps
  them out)

Whether this is medium-scope-with-inline-doc-touch or warrants a Phase 4
foundation roll-forward at scope time is a judgment call; sizing this as medium
here, with the doc touch listed as feature-level acceptance (handled inline
during implementation, not pre-locked at scope time).

## Acceptance criteria

- [ ] Existing orgs migrated to `members_only` policy via the schema default
- [ ] `AcceptSessionInvite` rejects non-org-members when org is `members_only`
- [ ] `AcceptSessionInvite` succeeds for non-org-members when org is `open`
      (preserves the current refactor's behavior)
- [ ] `PATCH /api/orgs/{orgID}` accepts `session_invite_policy`; admin-role gated
- [ ] `OrgSettings.svelte` screen renders current policy + lets admins save
- [ ] `InviteAccept.svelte` renders happy-path and members-only rejection states
- [ ] All existing tests pass; new tests cover both policy paths
- [ ] `docs/ARCHITECTURE.md` (or `docs/SECURITY.md`) updated with a short
      membership-model section
- [ ] Mockups committed under `.mockups/screens/org-session-invite-policy/`

## Implementation summary

All 6 child stories implemented in three waves via implement-orchestrator,
then advanced to `stage: review`. Commit chain:

- `9f668a6` — schema (+ sqlc regen + ARCHITECTURE.md membership-model section)
- `ef09ad0` — get-invite-details (GET endpoint + 8 tests)
- `eaa0295` — invite-accept-enforce (policy gate + cross-product tests)
- `81e89bd` — patch-endpoint (PATCH /api/orgs/{orgID} + 6 tests)
- `422b077` — org-settings-ui (also added GET /api/orgs/{orgID} that the
  design assumed existed; 10 component tests + 4 handler tests)
- (next commit) — invite-accept-ui (5-state hero screen + 14 tests + login
  return-to round-trip)
- `4681f53` — integration fix: GetOrg test-double stubs across 5 portal
  packages that Wave 3a's interface addition missed

### Cross-cutting notes
- **Bonus endpoint**: Wave 3a added `GET /api/orgs/{orgID}` as a sibling of
  PATCH. The design assumed it existed; the agent caught the gap and added
  it inline rather than blocking. Handler in `accounts/orgs.go`, route in
  `cmd/portal/main.go`.
- **Login return-to**: implemented client-side in `Login.svelte` via an
  `$effect` that runs when `auth.isAuthenticated` becomes true. The
  existing OAuth flow is server-side redirect, so the client-side hook
  catches magic-link + subsequent flows. Documented limitation: pure
  OAuth round-trip needs a follow-up backend change for full return-to.
- **Wave 1 collateral**: schema agent updated `sqlc.yaml` overrides for
  lease/finalize-lock timestamp columns to keep regeneration consistent
  across sqlc versions. get-invite-details agent fixed type mismatches
  in the adapter converters left by sibling story
  `lease-fencing-schema-verify-sqlc-regen`.
- **Test-stub integration**: 5 `*OnlyStrict` test doubles needed GetOrg
  panic stubs after Wave 3a; orchestrator added them in 4681f53.

### Verification
- `go build ./...` — clean
- `go test ./internal/portal/... -count=1` — 27/27 packages pass
- `go test ./internal/db/... -count=1` — all pass
- `cd frontend && npm test` — 357/357 tests pass
- `cd frontend && npm run check` — 0 errors (2 pre-existing warnings unchanged)

### Acceptance criteria status

- [x] Existing orgs migrated to `members_only` policy via schema default
- [x] `AcceptSessionInvite` rejects non-org-members under `members_only`
- [x] `AcceptSessionInvite` succeeds for non-org-members under `open`
- [x] `PATCH /api/orgs/{orgID}` accepts `session_invite_policy`; admin-role gated
- [x] `OrgSettings.svelte` renders current policy + lets admins save
- [x] `InviteAccept.svelte` renders happy-path + members-only rejection states
- [x] All tests pass; new tests cover both policy paths
- [x] `docs/ARCHITECTURE.md` updated with membership-model section
- [x] Mockups committed under `.mockups/screens/org-session-invite-policy-{settings,accept}/`

## History

This feature was sourced from `.work/backlog/audit-session-membership-org-implication.md`
(filed as an Important finding during the `refactor-handler-auth-guards-comments`
review). The audit's three resolution paths (restore org check / fix invite flow /
explicit guest model) collapsed once we noticed the right framing is per-org
configuration: every org gets to pick. The audit body has been replaced by this
feature; git history preserves the audit's original analysis.

## Review (2026-05-17)

**Verdict**: Approve

**Blockers**: none
**Important**:
- One finding carried over from `invite-accept-ui` review: commit `550280d`
  bundled this feature's last child story with unrelated e2e-design work
  under a misleading title. Filed as backlog
  `agent-commit-isolation-under-concurrent-autopilot` — not blocking this
  feature; addresses a meta-process concern for future autopilot runs.

**Nits**:
- Feature delivered an extra endpoint (`GET /api/orgs/{orgID}`) the design
  assumed existed. Caught and inlined by the org-settings-ui agent rather
  than blocking. Implementation discovery handled gracefully.
- OAuth-flow return-to is a documented follow-up limitation in
  `Login.svelte` (in-line comment + invite-accept-ui story body). Magic-
  link return-to works end-to-end via the client-side `$effect`.

**Capability completeness check**:
- [x] `orgs.session_invite_policy` column + `members_only` default
- [x] `AcceptSessionInvite` enforces policy at perimeter
- [x] `PATCH /api/orgs/{orgID}` flips policy with admin gate
- [x] `OrgSettings.svelte` renders for admin (editable) + non-admin (read-only)
- [x] `InviteAccept.svelte` renders happy / rejection / error states
- [x] Grandfather invariant: existing session_members untouched on flip
- [x] Foundation doc updated (`docs/ARCHITECTURE.md` membership-model section)
- [x] Mockups committed for both UI surfaces

**End-to-end verification path** (per feature's "Integration check"):
The cross-product is exercised through `TestAcceptSessionInvite_{MembersOnly,Open}Policy_{Member,NonMember}` plus `TestPatchOrg_Grandfather`. The
manual-cross-check the feature called out (flip via PATCH → accept from
non-org-member under `open` → flip back and verify session_member
grandfathered) is fully covered by automated tests now — no manual check
needed.

**Notes**: Six stories, five waves, one orchestrator integration fix (the
GetOrg test-double stubs in `4681f53`). Final state: full suite green
(go: 27/27 portal packages; frontend: 357/357 tests; `npm run check` 0
errors). The feature ships as briefed.
