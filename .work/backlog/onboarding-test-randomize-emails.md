---
id: onboarding-test-randomize-emails
kind: story
stage: drafting
tags: [e2e-test, testing]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# `onboarding_test.go` uses hardcoded emails

## Finding

`tests/e2e/golden/onboarding_test.go > TestOnboardingMagicLink` uses
`alice@example.com` and `bob@example.com` as the two test identities.

The mailhog fixture creates a fresh container per `Start()` call, so within
the same `go test` process the test is isolated. But if two e2e processes
share a MailHog container (e.g., via a shared docker-compose stack in a
developer's dev loop), `LatestMessageTo("alice@example.com")` could return a
message from a sibling process.

## Suggested fix

Randomize the emails at test start:

```go
suffix := randHex(4) // e.g. "a1b2"
aliceEmail := "alice-" + suffix + "@example.com"
bobEmail := "bob-" + suffix + "@example.com"
```

Apply the same pattern to any future golden-path story that uses MailHog.

## Acceptance criteria

- [ ] `TestOnboardingMagicLink` uses randomly-suffixed emails
- [ ] No new dependencies required (use `crypto/rand` from stdlib)
- [ ] A short helper `randEmail(prefix)` in the mailhog fixture or test
      package extracts the pattern for reuse by sibling golden-path
      stories
