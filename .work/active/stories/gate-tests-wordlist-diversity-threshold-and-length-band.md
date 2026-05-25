---
id: gate-tests-wordlist-diversity-threshold-and-length-band
kind: story
stage: implementing
tags: [testing, portal, playground]
parent: feature-test-spec-drift-and-coverage
depends_on: []
release_binding: null
gate_origin: tests
created: 2026-05-24
updated: 2026-05-25
---

# `TestPick_Diversity` threshold weak — would pass with a 256→180 wordlist shrink

## Priority
Low

## Spec reference
Item: `story-playground-server-hardening-wordlist-dedup`

Acceptance criterion: Story AC: "the existing diversity test threshold (900/1000 distinct picks over the joint adj×animal space) continues to pass."

## Gap type
complementary coverage

## Suggested test
Tighten `TestPick_Diversity` to also assert wordlist lengths are within an
expected band (≥150 entries each) so accidental wordlist truncation is
flagged early.

## Test location (suggested)
`internal/portal/playground/wordlist/wordlist_test.go`

## Implementation

Two-file change:

**`internal/portal/playground/wordlist/wordlist.go`** — add two accessor
functions after the existing `Pick()`:
```go
// AdjCount returns the number of adjective entries in the embedded wordlist.
// Used in tests to verify the wordlist has not been accidentally truncated.
func AdjCount() int { return len(adjs) }

// AnimalCount returns the number of animal entries in the embedded wordlist.
func AnimalCount() int { return len(animals) }
```

**`internal/portal/playground/wordlist/wordlist_test.go`** — add a new
test after `TestWordlistsNonEmpty`:
```go
func TestWordlistLengthBand(t *testing.T) {
    const minEntries = 150
    if n := wordlist.AdjCount(); n < minEntries {
        t.Errorf("adjectives wordlist has %d entries; want >= %d", n, minEntries)
    }
    if n := wordlist.AnimalCount(); n < minEntries {
        t.Errorf("animals wordlist has %d entries; want >= %d", n, minEntries)
    }
}
```

Current counts: adjectives 177, animals 182. The 150 threshold is ~85% of the
smaller list — meaningful guard against truncation, not a brittle exact count.

Do NOT change `TestPick_Diversity`'s 900/1000 threshold.
