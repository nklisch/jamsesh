---
id: gate-tests-postgres-lease-ci-wiring
kind: story
stage: drafting
tags: [testing, infra, portal]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: tests
created: 2026-05-18
updated: 2026-05-18
---

# Postgres lease manager integration tests gated on `JAMSESH_TEST_PG_DSN` with no CI invocation visible

## Priority
Medium

## Spec reference
Item: `epic-cloud-native-deploy-lease-fencing-postgres`
Acceptance criterion: integration tests gated on `JAMSESH_TEST_PG_DSN`;
skip cleanly without.

## Gap type
test-integrity. The skip discipline is correct, but the 8 critical-path
assertions (acquire success, monotonic fencing tokens, ErrAlreadyHeld,
Lost-on-backend-terminate, Release idempotency, heartbeat liveness,
Release-after-Lost idempotency, collision check) ONLY run when an
operator has set the env var. There's no observable CI step in the
bundle that sets `JAMSESH_TEST_PG_DSN`.

## Suggested test
Either (a) wire a CI step that runs
`JAMSESH_TEST_PG_DSN=$dsn go test ./internal/portal/lease/...`
against a test Postgres, OR (b) restructure to use `testcontainers-go`
so the test is self-bootstrapping.

## Test location (suggested)
`internal/portal/lease/postgres_test.go` + `.github/workflows/*` and new
fixture wiring.
