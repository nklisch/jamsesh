---
id: anon-bearer-transactional-rollback-test-coverage
kind: story
stage: implementing
tags: [testing, tokens]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# Add a real transactional-rollback test for IssueAnonymousSessionBearer

## Brief

The current `TestIssueAnonymousSessionBearer_TransactionalRollback` test in
`internal/portal/tokens/anon_bearer_test.go` does NOT exercise transactional
rollback. Its body only invokes the issuance helper with an empty `sessionID`,
which is rejected by the pre-tx validation guard — no DB calls are made at
all, so there's nothing for `WithTx` to roll back. The test name is a lie.

The Phase 4 acceptance criterion in
`feature-epic-ephemeral-playground-anon-bearer` explicitly called for:

> Transactional rollback: if account creation succeeds but bearer creation
> fails (e.g., via a wrapping store injecting an error), no account row is
> left behind

This is the test we actually need. Options:

1. Wrap the store with a decorator whose `CreateAnonymousBearer` returns an
   error after `CreateAnonymousAccount` succeeds, then assert no anonymous
   account row was committed.
2. Cause a uniqueness/constraint failure on the bearer insert (e.g., pre-seed
   a row that collides on `token_hash`) — harder because the hash is
   randomly generated.

Option 1 is cleanest. Rename the existing misnamed test (it can stay as
`_EmptySessionID_NoDBCalls` since the no-DB-calls assertion is its real
value), and add a new test that actually exercises the rollback path.

## Source

Surfaced during review of
`feature-epic-ephemeral-playground-anon-bearer`. Filed under the
test-integrity discipline in CLAUDE.md ("A failing test that documents why
it fails ... is more honest than a green test that lies").
