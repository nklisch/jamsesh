---
id: gate-tests-anon-bearer-display-name-roundtrip-edge-cases
kind: story
stage: done
tags: [testing, portal, tokens]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: tests
created: 2026-05-24
updated: 2026-05-24
---

# `IssueAnonymousSessionBearer` display-name round-trip edge cases not covered

## Priority
Medium

## Spec reference
Item: `feature-epic-ephemeral-playground-anon-bearer`

Acceptance criterion: Unit 4 AC: "After issuance, `Validate(ctx, rawToken)` returns the new `*store.Account` with `IsAnonymous: true, DisplayName: 'amber-otter'`."

## Gap type
partial coverage — round-trip exists but not for collision-fallback handles (`quiet-otter-x1`, `swift-heron-a3f2`)

## Suggested test
Table-driven over `[]string{"amber-otter", "quiet-fox", "swift-heron-a3f2"}`
confirming round-trip preserves the suffix.

## Test location (suggested)
`internal/portal/tokens/anon_bearer_test.go`

## Implementation notes

Added five test functions to `internal/portal/tokens/anon_bearer_test.go`:

1. **`TestIssueAnonymousSessionBearer_DisplayNameRoundTrip`** — table-driven over
   `["amber-otter", "quiet-fox", "swift-heron-a3f2", "quiet-otter-x1", "bold-crane-3b9e42"]`;
   covers both plain two-word handles and collision-fallback handles with hex suffixes.

2. **`TestIssueAnonymousSessionBearer_DisplayNameRoundTrip_Unicode`** — Latin-extended,
   CJK, and en-dash characters; pins that SQLite TEXT is UTF-8-native and does not
   mangle multi-byte names.

3. **`TestIssueAnonymousSessionBearer_DisplayNameRoundTrip_MaxLength`** — 255-character
   ASCII name; pins that the storage layer does not silently truncate (no length
   constraint on the `display_name TEXT NOT NULL` column).

4. **`TestIssueAnonymousSessionBearer_LeadingTrailingWhitespace`** — confirms the
   service stores whitespace-padded handles verbatim (no silent `TrimSpace`).

5. **`TestIssueAnonymousSessionBearer_WhitespaceOnlyNickname_CurrentBehaviour`** —
   documents that whitespace-only nicknames (`"   "`) are currently accepted and stored
   as-is; includes a comment directing future contributors to add a `TrimSpace` guard
   in `service_impl.go` if the product ever decides to reject them.

No production bugs were surfaced. All tests pass (`go test ./internal/portal/...` green).

## Review notes

Approve. Five tests cover collision-fallback handles, unicode, max-length,
whitespace padding, and whitespace-only nickname (current behaviour documented
honestly per the project's "test the truth" rule). All exercise the real
`Validate` round-trip through a real SQLite store. Tests pass.
