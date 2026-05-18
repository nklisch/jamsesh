---
id: lease-fencing-schema-verify-sqlc-regen
kind: story
stage: implementing
tags: [chore, portal]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Verify sqlc regen against hand-written lease schema files

## Brief

In `epic-cloud-native-deploy-lease-fencing-schema` (commit 6264542),
`sqlc` was not installed in the implementation environment. The agent
hand-wrote the generated files (`internal/db/pgstore/leases.sql.go`,
`internal/db/sqlitestore/leases.sql.go`, plus additions to `models.go`
and `querier.go`) following the exact patterns of existing generated
code.

Risk: if `make generate-db` (which runs `sqlc generate`) is later run by
a developer with sqlc installed, the regen could produce diffs against
the hand-written output — type signatures, parameter struct names,
import ordering — anywhere sqlc's output deviates from the agent's
guess at the canonical pattern.

## Fix

Run `make generate-db` on a machine with sqlc v1.31.x installed
(matching the version in any existing CI / docs). Compare diffs against
the committed hand-written files. Reconcile:

- If diffs are purely cosmetic (comments, whitespace, import order),
  commit the regenerated files.
- If diffs are semantically different (different parameter struct
  names, different query method signatures), the hand-written code is
  wrong — fix or replace, then verify `go build ./... && go test ./...`
  still passes.

## Acceptance criteria

- [ ] `make generate-db` runs cleanly with no errors
- [ ] Diff between hand-written and sqlc-generated files for lease
  queries is reviewed
- [ ] Any semantic differences are reconciled
- [ ] `go build ./... && go test ./...` passes post-regen

## Notes

Filed during review of
`epic-cloud-native-deploy-lease-fencing-schema`. Not blocking the
story's advancement to done — the hand-written code compiles, passes
tests, and matches established codebase patterns. This is a safety
follow-up to ensure no drift accumulates before downstream consumers
(`epic-cloud-native-deploy-lease-fencing-postgres`) build on top.
