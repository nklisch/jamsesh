---
id: audit-session-membership-org-implication
kind: story
stage: drafting
tags: [security, portal]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Audit: does session membership imply org membership?

## Why this exists

The refactor `refactor-handler-auth-guards` (helpers-and-sessions + comments + accounts-tokens) replaced the
historical pattern `GetOrgMember(...) + GetSessionMember(...)` in some
handlers with a single `handlerauth.RequireSessionMember(...)` call.

The refactor's implementing agents asserted "session membership implies
org membership in this system" to justify dropping the explicit
org-membership check. The review of `refactor-handler-auth-guards-comments`
verified that the assertion is **not currently enforced** by the
invite-accept flow:

- `internal/portal/sessions/invites.go:AcceptSessionInvite` adds an
  `AddSessionMember(...)` row but does NOT add a corresponding
  `AddOrgMember(...)` row.
- Therefore an account invited by email to a session can become a
  `session_member` without ever being added to `org_members`.

Pre-refactor, the explicit `GetOrgMember` check in comments/handlers.go
(and other places) would 403 such an account. Post-refactor, they pass
through to operate on session comments.

This may be:

1. **A system-design improvement** — session invites should grant
   session-scoped access; org-membership is for org-level operations only.
   The pre-refactor org check was over-restrictive.
2. **A latent security regression** — the invite-accept flow should
   auto-add the account to `org_members` (per the multi-tenancy model in
   `docs/ARCHITECTURE.md` that asserts everything is org-scoped). The
   pre-refactor check was defending against the missing add.

The autopilot review can't resolve this — it's a system-design call. Both
readings are defensible against the foundation docs.

## What this audit needs to determine

1. **Intent**: should accepting a session invite grant the invitee org
   membership automatically, or only session membership?
2. **Resource scope**: which API operations should require org membership
   vs session membership only? Comments specifically — is that an org
   resource or a session resource for membership purposes?
3. **Cross-org sessions**: is the model "sessions live inside one org and
   only that org's members can join", or "sessions can have cross-org
   guests"? The foundation docs lean toward (a), but the email-invite
   flow accommodates (b).
4. **Decision**: either (a) restore an org-membership check (in
   `RequireSessionMember` or in handlers) OR (b) update the invite-accept
   flow to auto-add `org_members` rows, OR (c) document explicitly that
   session invitees DO NOT need to be org members.

## Affected handlers (post-refactor — only session-member checked)

- `internal/portal/comments/handlers.go`: `CreateComment`, `ListComments`,
  `ResolveComment`
- `internal/portal/sessions/handler.go`: `PatchSession`, `AbandonSession`
  (note: these are creator-only after the session-member check, so the
  exposure surface is narrower)

## Affected flow

- `internal/portal/sessions/invites.go:AcceptSessionInvite` (line 212):
  `AddSessionMember` without a parallel `AddOrgMember`

## Suggested resolution paths

- **Path A** — restore org check: add `RequireOrgMember` upstream of
  `RequireSessionMember` in `comments/handlers.go`, OR augment
  `RequireSessionMember` to ALSO call `GetOrgMember` (cheap; one extra
  query). This brings back the pre-refactor behavior.
- **Path B** — fix invite flow: have `AcceptSessionInvite` add an
  `org_members` row (role `guest`?) before adding the `session_member`.
  This makes the new code's assumption hold.
- **Path C** — explicit cross-org guest model: document that session
  invitees are session-scoped guests without org membership. Audit every
  org-scoped query that uses `org_id` from a session context to ensure
  it can't be exploited to read org-wide data.

## Acceptance for this audit

- Confirmed answer to each "what this audit needs to determine" question
- A decision on Path A / B / C
- Implementation item(s) created to execute the chosen path
- Foundation doc (`docs/ARCHITECTURE.md` or a new section in
  `docs/SECURITY.md`) updated to document the membership model

## Severity

Important, not blocking. The current behavior may already be acceptable;
the audit's job is to make that explicit.
