---
id: feature-test-spec-drift-and-coverage
kind: feature
stage: drafting
tags: [testing, portal, ui, spec]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-25
updated: 2026-05-25
---

# Test coverage + spec-drift gaps

## Brief

Cluster of pure-test-addition / test-resilience items surfaced by recent
`gate-tests` runs and post-implementation reviews. No production code
changes — these tighten existing tests, add missing branches, retrofit a
per-dialect harness, and check the spec-drift detector against non-default
cwd. Bounded; no architectural shift; no foundation-doc impact.

## Member stories

- `gate-tests-event-discriminator-triad-completeness` —
  extend the spec-drift test to cross-check `discriminator.mapping` and
  the matching `oneOf` payload schemas
- `gate-tests-spec-drift-cwd-resilience` —
  sub-test invoking the spec-drift comparison from a `t.Chdir(t.TempDir())`
- `gate-tests-wordlist-diversity-threshold-and-length-band` —
  tighten the diversity threshold and add wordlist-length-band assertion
- `gate-tests-joinerpicker-410-race-recovery` —
  410 race-recovery test against double-click on JoinerPicker
- `idea-sessions-handler-tests-per-dialect-retrofit` —
  mechanical retrofit: wrap every `internal/portal/sessions/*_test.go`
  test in the per-dialect `storetest.Stores(t)` pattern (mirrors the
  playground retrofit in commit f59e45f)

## Approach (high level)

All five are independent. The sessions per-dialect retrofit is the
largest piece (65+ tests across 7 files, purely mechanical). The other
four are bounded additions.
