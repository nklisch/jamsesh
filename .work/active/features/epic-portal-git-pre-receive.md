---
id: epic-portal-git-pre-receive
kind: feature
stage: drafting
tags: [portal, security]
parent: epic-portal-git
depends_on: [epic-portal-git-storage]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Portal Git — Pre-Receive Policy

## Brief

The policy-enforcement library invoked by the smart-HTTP receive-pack handler
BEFORE the pushed pack is accepted into the bare repo. Validates every aspect
of a push and either accepts (handler proceeds with `git-receive-pack`) or
rejects with a structured git-protocol error message listing offending
commits, paths, or refs.

**Validations** (per `docs/SECURITY.md > Git push authorization` and
`docs/SPEC.md > Hard constraints`):

- **Ref namespace**: the ref being updated must be in the authenticated
  user's namespace (`jam/<session>/<user>/*`). Sole exception: the
  session-creation base-push (first push to `jam/<session>/base` by the
  session creator, when the bare repo has no refs yet).
- **No force-push on shared refs**: pushes to `base` (after creation) and
  `draft` are rejected. Force-pushes are detected by checking that the
  new sha is a descendant of the old sha for the ref.
- **Commit trailer presence**: every commit in the pack must carry
  `Jam-Session: <session-id>`, `Jam-Turn: <turn-number>`, and
  `Jam-Author: <user-handle-or-id>`. Optional trailers (`Resolves-Conflict`,
  `Auto-Merger`, `Source-Commit`) are recognized but not required.
- **Writable scope**: for every commit, every changed path must match
  at least one glob from the session's declared writable scope. Path
  walking uses `go-git` to enumerate the commit tree diff.
- **Pack size limit**: 50 MB per push by default (configurable). Rejected
  with `push.size_limit` if exceeded.

**Execution model** (locked at epic-design): validation runs in-process in
Go in the receive-pack HTTP handler, NOT via a shell `hooks/pre-receive`
script that calls back into the portal. The handler reads the pushed pack
into a temp area (or streams it into a temporary objects directory),
walks the proposed updates with `go-git`, runs validations, then either
hands off to `git-receive-pack` to apply OR returns the git-protocol error.

**Rejection message format**: git-protocol-compatible (the receive-pack
report-status format) so `git push` displays them inline. Structured
content for the portal API: an error envelope listing offending
commits/paths/refs per the `docs/PROTOCOL.md > HTTP error contract`
codes (`push.scope_violation`, `push.ref_namespace_violation`,
`push.missing_trailer`, `push.size_limit`, `push.force_push_rejected`).

Does NOT cover the HTTP handler itself (`smart-http` feature). Does NOT
cover event emission after acceptance (`post-receive`).

## Epic context

- Parent epic: `epic-portal-git`
- Position in epic: meatiest feature in the epic; consumed by smart-http
  as a library. Storage feature is its only intra-epic dep (for bare
  repo opening and session lookups).

## Foundation references

- `docs/SECURITY.md` — Git push authorization (the canonical validation
  list), Trust model for participants (Mistaken or buggy participants)
- `docs/SPEC.md` — Ref structure, Hard constraints (multi-tenant, writable
  scope), Session shape
- `docs/PROTOCOL.md` — Commit trailer conventions (required vs optional
  trailers), HTTP error contract (`push.*` codes)
- `docs/ARCHITECTURE.md` — Data flow: a turn > Pre-receive validates

## Inherited epic design decisions

- **Execution model**: in-process Go validation in the HTTP handler,
  using `go-git` for object walking. No shell-hook callback pattern.
- **Pack size limit**: 50 MB default, configurable, `push.size_limit`
  error code.

## Decomposition risks

- Pre-receive is the highest-risk feature in this epic. Wire-protocol
  validation has been a long tail of edge cases historically. Mitigation:
  use `go-git` rather than rolling our own pack parser; lean on its
  object-walk APIs; design pass produces a thorough test plan covering
  the trailer / scope / namespace / force-push / size matrix.

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->
