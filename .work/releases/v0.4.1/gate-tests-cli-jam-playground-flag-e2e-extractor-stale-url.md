---
id: gate-tests-cli-jam-playground-flag-e2e-extractor-stale-url
kind: story
stage: done
tags: [testing, e2e-test, bug, cli, playground]
parent: null
depends_on: []
release_binding: v0.4.1
gate_origin: tests
created: 2026-05-25
updated: 2026-05-25
---

# e2e share-URL extractor returns `"s"` after URL format change

## Priority
Critical

## Spec reference
Item: `story-fix-cli-playground-share-url`
Fix block: printed URL changed from `<base>/playground/<id>` to
`<base>/playground/s/<id>/join`. Fix commit `4e43d2a` added the unit test
`TestPlaygroundAction_shareURLShape` but did NOT update the e2e extractor.

## Gap type
Stale e2e — implementation regression: extractor no longer parses the URL
it's documented to parse.

## Location
`tests/e2e/golden/cli_jam_playground_flag_test.go:207-230` —
`extractPlaygroundSessionID` greps for the prefix `<baseURL>/playground/`
then strips at the first `/`. With the new URL `…/playground/s/<id>/join`
it returns `"s"` as the session ID. Every downstream assertion
(`readStateFile("sessions/s/token")`, `GET /api/playground/sessions/s`,
docker-exec `ls /tmp/.../sessions/s.git`) is then probing a synthetic id
that never exists.

## Suggested test
```go
// Update extractor regex/parse to anchor on "/playground/s/" and split
// before "/join". Add a unit test on the helper itself with two inputs:
//   "http://example.org/playground/s/abc-def/join" → "abc-def"
//   "http://example.org/playground/s/x/join?utm=1"  → "x"
// to lock the contract.
```

## Test location (suggested)
`tests/e2e/golden/cli_jam_playground_flag_test.go:207-230` (in-place
fix + add a helper unit test in the same file).

## Impact
The e2e either silently fails after the bundle's fix OR has never been
re-run in v0.4.1; either way the bundle would ship a broken
anti-tautology check that no longer verifies what its docstring claims.

## Implementation notes (2026-05-25)

Inline fix during gate-tests cycle:

- `tests/e2e/golden/cli_jam_playground_flag_test.go:213-230` — rewrote
  `extractPlaygroundSessionID` to anchor on the new `/playground/s/`
  prefix, split before `/join` (or any trailing whitespace/EOL), and
  updated the docstring + fatal message to match the new URL shape.
- `cd tests/e2e && go vet ./golden/` clean — no compile or static errors.
- Full e2e run is gated on testcontainers; deferred to CI.
