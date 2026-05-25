---
id: gate-tests-wordlist-diversity-threshold-and-length-band
kind: story
stage: done
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

## Implementation notes

- `internal/portal/playground/wordlist/wordlist.go`: added two exported
  accessors — `AdjCount() int` and `AnimalCount() int` — returning the
  length of the respective embedded slices. Minimal public-API addition
  needed because the test file is `package wordlist_test` (external).
- `internal/portal/playground/wordlist/wordlist_test.go`: added
  `TestWordlistLengthBand` asserting each list is `>= 150` entries. 150
  is ~85% of the smaller list (animals, ~182), providing a guard against
  accidental truncation without being brittle to small editorial trims.
- The existing `TestPick_Diversity` threshold (>= 900 distinct of 1000
  picks) is left as-is. With 150x150 = 22.5k combinations as the floor,
  900/1000 distinct is still well within reach — the diversity test
  remains a useful smoke check.

Verified: `go test ./internal/portal/playground/wordlist/... -count 1 -v` →
all tests pass (current state: 239 adjectives, 182 animals).

## Review (2026-05-25)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: 150 floor (~85% of the smaller list) is the right calibration — guards against truncation without breaking on small editorial trims. Error messages name the likely cause ("accidental truncation?") which speeds future diagnosis. Two-line public-API accessor addition is minimal and well-documented.
