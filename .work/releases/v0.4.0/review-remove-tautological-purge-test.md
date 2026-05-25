---
id: review-remove-tautological-purge-test
kind: story
stage: done
tags: [testing, portal, playground, cleanup]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# Remove tautological `TestWorker_PurgesTombstonesAfterTTL` superseded by worker-cadence test

## Source
Spawned from review of
`gate-tests-tombstone-purge-via-worker-not-store-tautology`.

## Context
The parent story's goal was to replace the tautological
`TestWorker_PurgesTombstonesAfterTTL` (which calls
`env.s.PurgeExpiredTombstones(ctx, now)` directly — proves the SQL works
but says nothing about the worker firing it on cadence) with
`TestWorker_PurgesTombstones_OnPurgeEveryTickInterval` (which drives the
full `worker.Run()` path).

The new test was added (and runs correctly). However, the old
tautological test was NOT removed — both now coexist in
`internal/portal/playground/worker_test.go` (lines 251 and 775).

## What to do
Delete `TestWorker_PurgesTombstonesAfterTTL` (lines 251-286 of
`internal/portal/playground/worker_test.go`). The new
`TestWorker_PurgesTombstones_OnPurgeEveryTickInterval` is the replacement
and the new in-file comment block (line 764) explicitly says
"replaces the existing TestWorker_PurgesTombstonesAfterTTL".

Verify the package still builds and the remaining purge/cadence tests
pass after removal.

## Implementation notes
Deleted `TestWorker_PurgesTombstonesAfterTTL` (lines 251–286) from
`internal/portal/playground/worker_test.go`. The replacement test
`TestWorker_PurgesTombstones_OnPurgeEveryTickInterval` (line 775 in
original, now renumbered) already exists and covers the worker-driven
purge path. The playground package continues to build and all tests pass
after removal.
