---
id: gate-tests-wordlist-empty-or-dashonly-corruption-resistance
kind: story
stage: review
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

## Implementation notes

Added `internal/portal/playground/wordlist/corruption_resistance_test.go` (internal/white-box package so `splitNonEmpty` is directly accessible). Five tests:

- `TestPick_NeverProducesEmptyOrDashOnly` — 10 000 iterations; asserts every handle is non-empty, has exactly one hyphen, and neither the adjective nor animal part is empty.
- `TestSplitNonEmpty_FiltersBlankAndWhitespaceLines` — verifies blank/whitespace lines are stripped.
- `TestSplitNonEmpty_EmptyInput` — empty, all-whitespace, and all-newline inputs return empty slice without panic.
- `TestSplitNonEmpty_DashOnlyLinesPassThrough` — documents the design boundary: `splitNonEmpty` is a blank-line stripper, not a word validator; dash-only entries pass through. Protection is the curated wordlist, not the parser.
- `TestSplitNonEmpty_TrimsLeadingTrailingSpacesFromEntries` — entries with surrounding whitespace are trimmed before they reach the slice.

All 8 tests (5 new + 3 pre-existing) pass. No production bugs found; the real wordlists contain no blank or dash-only lines.
