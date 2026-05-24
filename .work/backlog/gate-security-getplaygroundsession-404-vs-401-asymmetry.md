---
id: gate-security-getplaygroundsession-404-vs-401-asymmetry
kind: story
stage: drafting
tags: [security, portal, playground, api]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: security
created: 2026-05-24
updated: 2026-05-24
---

# `GetPlaygroundSession` returns 404 vs 401 in different branches, allowing a holder of a valid anon bearer to probe session existence across the playground org

## Severity
Low

## Domain
API Security

## Location
`internal/portal/playground/handler.go:325-353`

## Evidence
```go
sess, err := h.Store.GetSession(ctx, ReservedOrgID, sessionID)
if err != nil {
    if errors.Is(err, store.ErrNotFound) {
        return openapi.GetPlaygroundSession404JSONResponse{...}, nil
    }
    // ...
}
// Verify the bearer's account is a member of this session.
_, err = h.Store.GetSessionMember(ctx, ...)
if errors.Is(err, store.ErrNotFound) {
    return openapi.GetPlaygroundSession401JSONResponse{...}, nil
}
```

A caller holding any valid playground anon bearer can iterate session IDs and
distinguish "session does not exist (404)" from "session exists, I'm not a
member (401)". The corresponding `/git/orgs/.../sessions/...` paths fold both
into 401 (`auth.go:82-85`), so this REST surface is the asymmetric one.
Session IDs are ULIDs (128 bits) so the practical attack value is low, but
the leak deviates from the documented "do not reveal session existence"
stance.

## Remediation direction
Reorder the handler to check session-membership first (or fold the 404 into
the 401 branch by always returning `auth.invalid_or_not_a_member`), matching
the git-handler's stance.
