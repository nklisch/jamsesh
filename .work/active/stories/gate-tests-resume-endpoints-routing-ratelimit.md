---
id: gate-tests-resume-endpoints-routing-ratelimit
kind: story
stage: implementing
tags: [testing]
parent: null
depends_on: []
release_binding: v0.5.0
gate_origin: tests
created: 2026-05-31
updated: 2026-05-31
---

# Resume endpoints mounting and rate limits are not covered through HTTP routing

## Priority
Critical

## Spec reference
Item: `epic-cli-browser-session-resume-portal-contract`
Acceptance criterion: "Unit 2: mounted under bearer middleware ... Rate-limited" and "Unit 3: mounted public + rate-limited + ignores ambient `Authorization`"

## Gap type
missing test for security/routing acceptance criterion

## Suggested test
```go
// Exercise POST /api/session-resumes and /api/session-resumes/exchange through
// the real router: mint requires bearer + rate limits; exchange is public,
// ignores Authorization, and has its own rate limit.
```

## Test location (suggested)
`cmd/portal/main_test.go`

