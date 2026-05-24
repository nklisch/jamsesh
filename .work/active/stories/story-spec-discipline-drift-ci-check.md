---
id: story-spec-discipline-drift-ci-check
kind: story
stage: review
tags: [portal, infra, testing]
parent: feature-spec-discipline
depends_on: [story-spec-discipline-audit-and-close-emit-vs-yaml-gaps]
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# CI test that asserts Go event-type constants match the openapi.yaml enum

## Brief

Add a Go test that prevents the same bug class that produced the
`playground.activity_reset` / `session.destroyed` spec gap. The test
reads `docs/openapi.yaml`, extracts the `EventEnvelope.type` enum
values, extracts the event-type string constants from the Go server,
and asserts the two sets match exactly.

Runs as a normal `go test` — no new CI infrastructure.

## Current state

Today, a developer who adds a new server-side event emission can
forget to update `docs/openapi.yaml` and CI will not notice. The
mismatch surfaces only when the frontend tries to consume the event
and finds no generated type. The autopilot run found exactly this
class of bug.

## Target state

A Go test at `internal/portal/events/spec_drift_test.go` (or
equivalent location near the canonical event-type list) that:

1. Reads `docs/openapi.yaml` and parses it via `gopkg.in/yaml.v3` or
   `github.com/getkin/kin-openapi` (which is already a transitive
   dep via oapi-codegen).
2. Extracts the string values under
   `components.schemas.EventEnvelope.properties.type.enum`.
3. Extracts the canonical Go event-type list. The exact source of
   truth depends on what the codebase has — either:
   - A `package events` (or similar) with declared string constants
     (`EventCommitArrived = "commit.arrived"`, etc.), OR
   - A scan over `events.Emit(ctx, ..., "<type>", ...)` call sites
     using `go/parser`.
   The depending story (`story-spec-discipline-audit-and-close-emit-vs-yaml-gaps`)
   will likely consolidate these into a package-level constants block
   as part of its server-emit cleanup. This story should rely on
   that consolidated list. If no constants block exists when this
   story is picked up, the implementer creates one.
4. Asserts both sets are equal. On mismatch, prints the diff
   (`only in Go: ...`, `only in YAML: ...`) and fails.

## Implementation shape

```go
// internal/portal/events/spec_drift_test.go
package events_test

import (
    "os"
    "sort"
    "testing"

    "gopkg.in/yaml.v3"

    "jamsesh/internal/portal/events"
)

func TestEventTypeConstants_MatchOpenAPIYAML(t *testing.T) {
    yamlBytes, err := os.ReadFile("../../../../docs/openapi.yaml")
    if err != nil { t.Fatalf("read openapi.yaml: %v", err) }
    // ... parse, extract enum, compare to events.AllTypes()
}
```

Path lookup: use a `runtime.Caller`-based or relative-path
discovery so the test still works when run from a different cwd.

## Acceptance criteria

- [ ] Test exists at the chosen location and runs under
      `go test ./internal/portal/events/...` (or wherever it lands).
- [ ] Test passes against the current state AFTER
      `story-spec-discipline-audit-and-close-emit-vs-yaml-gaps` lands.
- [ ] Inject a deliberate mismatch (add a fake constant in the test
      package locally; verify the test fails with a clear diff
      message; remove). Document the failure-mode output in
      implementation notes.
- [ ] `go build ./...` and `go test ./...` clean.

## Risk

**Low.** Pure-additive test. No production code changes.

## Rollback

`git revert` the implementation commit.

## Notes

The test is intentionally not a build-tag-gated thing — it runs in
the normal test suite so CI catches drift on every PR. Reading
`docs/openapi.yaml` from a test is unusual but acceptable: it's a
checked-in artifact at a stable repo-relative path.

## Implementation notes

### Path chosen

**Path B** — canonical `var AllTypes []string` in
`internal/portal/events/types.go`. Path B was chosen over the AST-walk
approach because it is simpler, explicit, and gives the constants
double duty as documentation of the full event surface. The test compares
this list bidirectionally against the YAML enum.

### Audit finding: `auto-merger.backpressure` was missed

The preceding story (`story-spec-discipline-audit-and-close-emit-vs-yaml-gaps`)
audited 13 event-type strings but missed `auto-merger.backpressure`,
which is emitted at `internal/portal/automerger/worker.go:352` and was
absent from the YAML enum. This story closed that gap:

- Added `auto-merger.backpressure` to `EventEnvelope.type` enum in
  `docs/openapi.yaml`.
- Added `AutoMergerBackpressurePayload` schema, `oneOf` entry, and
  `discriminator.mapping` entry.
- Reran `go generate ./internal/api/openapi/...`; codegen produced
  `AutoMergerBackpressure` constant, `AutoMergerBackpressurePayload` struct,
  and accessor methods in `internal/api/openapi/server.gen.go`.

### Files added/modified

- `internal/portal/events/types.go` — new; defines `var AllTypes []string`
  with all 15 event-type strings.
- `internal/portal/events/spec_drift_test.go` — new; test
  `TestEventTypeConstants_MatchOpenAPIYAML`.
- `docs/openapi.yaml` — added `auto-merger.backpressure` to enum, oneOf,
  discriminator mapping, and new `AutoMergerBackpressurePayload` schema.
- `internal/api/openapi/server.gen.go` — regenerated by `go generate`.

### Failure-mode output observed during injection tests

**Injection 1** — added `"fake.event.for.test"` to `AllTypes`:

```
--- FAIL: TestEventTypeConstants_MatchOpenAPIYAML (0.00s)
    spec_drift_test.go:74: events.AllTypes and the docs/openapi.yaml EventEnvelope.type enum are out of sync.

        Only in Go (events.AllTypes) — add to docs/openapi.yaml or remove from AllTypes:
          + fake.event.for.test

        Resolution: see .claude/skills/patterns/spec-driven-event-types.md
```

**Injection 2** — removed `"auto-merger.backpressure"` from `AllTypes`
(simulating a YAML-only type):

```
--- FAIL: TestEventTypeConstants_MatchOpenAPIYAML (0.01s)
    spec_drift_test.go:74: events.AllTypes and the docs/openapi.yaml EventEnvelope.type enum are out of sync.

        Only in YAML enum — add to events.AllTypes or remove from docs/openapi.yaml:
          - auto-merger.backpressure

        Resolution: see .claude/skills/patterns/spec-driven-event-types.md
```

Both directions produce an unambiguous diff with actionable resolution guidance.

### Verification

- `go build ./...` — clean
- `go test ./internal/portal/events/...` — pass
- `go test ./...` — all packages pass (59 packages)
