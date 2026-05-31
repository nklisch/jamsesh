---
id: e2e-cloud-native-multipod-suite-red-fuzz
kind: feature
stage: drafting
tags: [portal, testing, bug]
parent: e2e-cloud-native-multipod-suite-red
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
---

# Fuzz suite stabilization

## Brief
The fuzz suite is red and uncharacterized. Characterize each harness's failure,
then resolve per the project test-integrity rules: genuine product
input-handling bugs are fixed (and, if a deep arc emerges, split out per the
epic's decomposition risks); stale seeds / harness drift / outdated assertions
are repaired in-session. Goal is a green fuzz suite under the 30s-per-harness
budget (`make test-fuzz`).

This feature is independent of the subsystem fixes and runs in parallel. Per the
parent epic's design decisions this is never-green stabilization — characterize
and root-cause forward from the current red state, no bisect.

## Epic context
- Parent epic: `e2e-cloud-native-multipod-suite-red`
- Position in epic: independent capability — parallel with all subsystem fixes;
  nothing depends on it.

## Foundation references
- `docs/ARCHITECTURE.md` — the surfaces each harness fuzzes
- Harnesses: `tests/e2e/fuzz/fencing_token_test.go`,
  `mcp_tool_input_test.go`, `object_storage_dsn_test.go`,
  `pack_manifest_test.go`, `playground_nickname_test.go`
- Run target: `make test-fuzz` (30s budget per harness; deeper runs via `-fuzztime`)
