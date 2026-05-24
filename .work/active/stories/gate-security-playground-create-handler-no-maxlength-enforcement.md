---
id: gate-security-playground-create-handler-no-maxlength-enforcement
kind: story
stage: review
tags: [security, portal, playground, validation]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: security
created: 2026-05-24
updated: 2026-05-24
---

# Playground create handler does not enforce OpenAPI `maxLength` constraints on `name`/`goal`/`nickname`

## Severity
Medium

## Domain
Input Validation & Injection

## Location
`internal/portal/playground/handler.go:89-114, 249-254`

## Evidence
```go
name := strings.TrimSpace(body.Name)
if name == "" {
    // ...
    name = "playground-" + strings.ToLower(shortID)
}
goal := body.Goal
scope := strings.TrimSpace(body.Scope)
```

And in Join:
```go
if req.Body != nil && strings.TrimSpace(req.Body.Nickname) != "" {
    candidates = []string{strings.TrimSpace(req.Body.Nickname)}
}
```

The OpenAPI spec at `docs/openapi.yaml:1531-1563` declares `name maxLength:200`,
`goal maxLength:4096`, `nickname maxLength:64`, but oapi-codegen's
`StrictServerInterface` only decodes the body without enforcing schema
constraints. The 1 MiB router body limit
(`internal/portal/router/router.go:170-173`) is the only ceiling, so a single
unauthenticated playground create can persist a ~1 MiB `goal`, ~1 MiB `name`,
or ~1 MiB anon-account `display_name` per request. With 3 creates/IP/hour and
IP rotation this is a storage-amplification / DB-bloat vector untouched by the
playground content-size cap (which only governs git pack bytes, not
session/account row sizes).

## Remediation direction
Validate `Name`, `Goal`, and `Nickname` lengths inside the handler against the
OpenAPI-declared maxima (return `session.invalid_name` /
`session.invalid_goal` / `auth.invalid_nickname` 400s), or wire a kin-openapi
RequestValidator middleware so all `/api/*` requests are checked against the
spec.

## Implementation notes

**What was already in place:** `JoinPlaygroundSession` already enforced the
nickname 2-24 char constraint (line 259) with a `playground.invalid_nickname`
400 response. The `anon-account display_name` mentioned in the story is the
nickname field — it was already covered. The actual gaps were `name` and `goal`
in `CreatePlaygroundSession`.

**What was added:** Two maxLength guards inserted in
`internal/portal/playground/handler.go` immediately after the `name` default
resolution and goal extraction, before the scope validation:

- `name` — `len([]rune(name)) > 200` → `400 session.invalid_name`
- `goal` — `len([]rune(goal)) > 4096` → `400 session.invalid_goal`

**Rune-based counting:** Length is measured in Unicode code points (`[]rune`
conversion), matching the spirit of the OpenAPI `maxLength` keyword (which
is character-count, not byte-count). This is consistent with how the
nickname validator counts `len(trimmed)` — though for the nickname the
`nicknameRE` restricts the alphabet to ASCII so rune and byte counts
coincide.

**Error codes:** Used `session.invalid_name` and `session.invalid_goal` — the
same namespace as the durable-session create handler's error codes for these
fields, so client error-branch logic can share a single handler for both
surfaces.

**Tests added** (`handler_test.go`):
- `TestCreatePlaygroundSession_NameMaxLength` — exactly-at-limit ASCII accepted,
  one-over rejected with `session.invalid_name`; plus exactly-at-limit unicode
  (é, 2 bytes/rune) accepted and one-over rejected — confirming rune-based not
  byte-based counting.
- `TestCreatePlaygroundSession_GoalMaxLength` — same structure for goal
  (4096 rune limit).

All `go test ./internal/portal/playground/...` and `go build ./...` pass.
