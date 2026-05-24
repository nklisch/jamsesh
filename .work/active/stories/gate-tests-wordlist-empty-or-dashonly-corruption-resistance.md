---
id: gate-tests-wordlist-empty-or-dashonly-corruption-resistance
kind: story
stage: drafting
tags: [testing, portal, playground]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: tests
created: 2026-05-24
updated: 2026-05-24
---

# Wordlist corruption-resistance test missing (empty or dash-only handle)

## Priority
Medium

## Spec reference
Item: `story-playground-server-hardening-wordlist-dedup`

Acceptance criterion: Story AC: "Wordlist embed loads at init; `wordlist.Pick()` returns N-distinct handles across 1000 calls." Design: "Curated 256x256 entries...deterministic across portal pods, refresh requires a release."

## Gap type
missing test for valid partition (the deterministic-across-pods guarantee)

## Suggested test
```go
func TestPick_NeverProducesEmptyOrDashOnly(t *testing.T) {
    // 10k iterations: never "" or "-" or ends/starts with "-".
    // Catches an accidental empty line in a wordlist file.
}
```

## Test location (suggested)
`internal/portal/playground/wordlist/wordlist_test.go`
