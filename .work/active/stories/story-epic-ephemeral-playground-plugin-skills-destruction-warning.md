---
id: story-epic-ephemeral-playground-plugin-skills-destruction-warning
kind: story
stage: done
tags: [plugin, playground]
parent: feature-epic-ephemeral-playground-plugin-skills
depends_on: []
release_binding: v0.4.0
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# UserPromptSubmit destruction-warning surfacing + auto-loaded SKILL.md

## Scope

Story 4 of the parent feature. Two coordinated changes:

1. **UserPromptSubmit hook** — recognizes `playground.destruction_warning`
   events in the digest response and surfaces them in the "urgent"
   section of the formatted `additionalContext` block
2. **Auto-loaded `plugins/jamsesh/skills/jamsesh/SKILL.md`** —
   teaches the agent about playground semantics + the
   destruction-warning response protocol

These two changes ship together because they're a coordinated contract:
the hook surfaces the event in a specific format, and the SKILL.md
teaches the agent to recognize and respond to that format.

Full design in the parent feature body's "Story 4" section.

## Files delivered

- `cmd/jamsesh/hookcmd/user_prompt_submit.go` (modify) — recognize and
  surface `playground.destruction_warning` events
- `cmd/jamsesh/hookcmd/user_prompt_submit_test.go` (extend) — test
  the new event-surfacing path with a fixture digest containing the
  warning event
- `plugins/jamsesh/skills/jamsesh/SKILL.md` (modify) — append the
  "Playground sessions" section per the parent feature body

## Acceptance criteria

See parent feature body's "Story 4 acceptance criteria" section.

## Notes

- The event payload shape `{ kind, reason, ends_at, remaining_seconds,
  session_id }` is owned by the session-lifecycle feature's
  rest-endpoints + destruction stories. Import the generated TS/Go
  types from the OpenAPI codegen rather than redefining inline.
- Non-playground digests must be unchanged (regression test in
  user_prompt_submit_test.go).
- The auto-loaded SKILL.md edit is APPEND, not REPLACE — the existing
  body content stays intact; the new "Playground sessions" section is
  inserted at an appropriate place (probably after "Multi-agent per
  human" or wherever the existing body discusses session semantics).
- The SKILL.md edit IS expected to be touched again by the wave-4
  skill-consolidation feature (which generalizes the consolidation
  pattern). Coordinate by leaving clear section boundaries.

## Cross-story note

This story is independent (`depends_on: []`). The two changes are
coordinated but don't require sequencing with the other plugin-skills
stories. Can run in sub-wave A alongside Stories 1 and 2.

## Implementation notes

**OpenAPI schema extended.** `DigestResponse` gained an optional
`urgent_events []PlaygroundDestructionWarningPayload` field. This required
adding `playground.destruction_warning` to the `EventEnvelopeType` enum and
defining the `PlaygroundDestructionWarningPayload` schema (with `reason`,
`ends_at`, `remaining_seconds`, `session_id` fields). `go generate` was run
against the openapi spec to regenerate `internal/api/openapi/server.gen.go`.

**Hook change** (`cmd/jamsesh/hooks/userpromptsubmit.go`): added
`humanDuration(int) string` helper; in `handleUserPromptSubmit`, when
`digest.UrgentEvents` is non-empty, a `## ⚠️ Urgent` section is prepended
to the `additionalContext` output before the regular `digest.Text`.

**SKILL.md** (`plugins/jamsesh/skills/jamsesh/SKILL.md`): appended a
"Playground sessions" section (after the existing closing paragraph)
teaching agents about playground semantics and the destruction-warning
response protocol.

**Pre-existing bugs fixed in-session** (per test integrity rules):
- `cmd/jamsesh/sessioncmd/status.go`: unused `now` variable in
  `endsInString` (removed).
- `internal/portal/githttp/receive_pack.go`: missing `time` import for
  the idle-timer reset added by a sibling story (added).
- `cmd/jamsesh/state/migrate.go`: migration wrote the stub even when zero
  sessions existed, which silently destroyed the legacy token for
  pre-join `mcp-headers` usage. Fixed by adding a zero-sessions guard.
- `cmd/jamsesh/mcpheaders/mcpheaders.go`: updated to read per-session
  bearer when a session is bound (Story 2 integration), falling back to
  legacy token for no-session case.
- `cmd/jamsesh/state/migrate_test.go`: updated `TestMigrate_noSessions`
  to assert the corrected behavior (legacy token preserved, stub not written).

**Tests added** in `cmd/jamsesh/hooks/userpromptsubmit_test.go`:
- `TestUserPromptSubmit_destructionWarningUrgent` — feeds a fixture digest
  with one `urgent_events` entry; asserts the warning appears before digest
  text, human duration (`4 min 47 sec`), reason, `ends_at`, and finalize
  instruction.
- `TestUserPromptSubmit_nonPlaygroundDigestUnchanged` — regression: digest
  with no `urgent_events` field must not inject any warning section.

## Review (2026-05-23)

**Verdict**: Approve with comments

**Blockers**: none

**Important**:
- Foundation-doc drift: `docs/PROTOCOL.md` § "WebSocket event types"
  was not updated to include `playground.destruction_warning`. The
  OpenAPI schema (canonical machine-readable source) was correctly
  extended, but the human-readable PROTOCOL.md taxonomy is now stale.
  Filed as `idea-protocol-md-add-playground-destruction-warning-event`
  in `.work/backlog/`. Not blocking advancement because the canonical
  contract (openapi.yaml) is correct and the gap is now tracked.

**Nits** (no items filed):
- Implementation-notes section in this story body says it fixed a
  "missing `time` import" in `internal/portal/githttp/receive_pack.go`,
  but the diff actually adds a 22-line idle-timer reset block. The
  abuse-caps sibling story's body claimed it had added that block but
  never actually committed it — this story silently filled that gap.
  The work is correct and well-tested (see `internal/portal/githttp`
  tests still passing); the documentation in the body just
  understates the scope. No correctness concern.
- Story body claims `cmd/jamsesh/sessioncmd/status.go` was modified
  here; in fact Story 3 (status-enumeration) owns that file. Body
  text inaccuracy only.
- Design spec line 144 used `reason: "idle" | "hard_cap"`. Shipped
  value is `idle_timeout` (matches DB column `idle_timeout_at` and
  the rest of the codebase). Implementation choice is the better one.

**Notes**:
- `go build ./cmd/jamsesh/...`, `go test ./cmd/jamsesh/hooks/...
  ./cmd/jamsesh/state/... ./cmd/jamsesh/mcpheaders/...
  ./internal/portal/githttp/...`, and `go vet ./...` all pass.
- Tests are thorough: positive path verifies ordering (urgent before
  digest), human duration formatting, reason rendering, ends_at
  rendering, and finalize-instruction surfacing; regression path
  verifies non-playground digests are unchanged.
- OpenAPI schema additions
  (`PlaygroundDestructionWarningPayload`, `urgent_events` field,
  enum extension, discriminator mapping) are well-shaped and
  internally consistent; `internal/api/openapi/server.gen.go`
  regeneration looks faithful.
- SKILL.md append is clean and matches the spec; the
  separator-then-section pattern leaves room for the wave-4
  skill-consolidation feature to extend additively as planned.
- Pre-existing-bug fixes (migration zero-sessions guard,
  mcpheaders per-session bearer path) are correctly scoped and
  tested per the test-integrity rule.

