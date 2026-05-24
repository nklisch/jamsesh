---
id: story-playground-server-hardening-wordlist-dedup
kind: story
stage: done
tags: [portal, playground, polish]
parent: feature-playground-server-hardening
depends_on: []
release_binding: v0.4.0
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# Playground wordlist has 62 duplicate adjectives

## Origin

Filed from review of
`story-epic-ephemeral-playground-session-lifecycle-rest-endpoints`.

## Problem

`internal/portal/playground/wordlist/adjectives.txt` contains 239 lines
but 62 of them are duplicates (e.g. `warm`, `wise`, `wide`, `tranquil`,
`vibrant`, `swift`, `sunny`, etc. each appear twice). The effective
adjective space is ~177 unique entries, not 239.

Verify with:
```
sort internal/portal/playground/wordlist/adjectives.txt | uniq -c | sort -rn | awk '$1>1' | wc -l
# → 62
```

Animals list is clean (0 duplicates).

## Impact

- Effective handle space is ~32k (177 × 182) instead of the designed
  ~43k (239 × 182), reducing diversity.
- Per `Pick()`, duplicates have 2× the chance of being selected vs other
  adjectives, biasing distribution.
- No correctness impact — `uniqueHandle` collision-retry still functions;
  the `tried` map skips already-tried candidates.

## Fix

Dedupe `adjectives.txt`. Optionally pad back to ~256 unique entries to
hit the design target. Re-run wordlist_test.go (the `TestPick_Diversity`
test will still pass — the threshold is 900/1000 distinct).

## Acceptance

- `sort adjectives.txt | uniq -c | awk '$1>1'` returns no rows.
- `wordlist_test.go` continues to pass.

## Design

Full spec is in the parent feature body under `## Implementation Units`
→ Unit 4 (wordlist dedup). Highlights:

- **Pure data change** on
  `internal/portal/playground/wordlist/adjectives.txt`. Mechanical fix:
  `sort -u adjectives.txt > new && mv new adjectives.txt` drops to
  177 unique entries.
- **Optional padding**: bring back up to ~256 unique entries with
  curated calm/positive adjectives (e.g. "balmy", "luminous",
  "polished") — alphabetically interleaved so the diff stays
  reviewable. Implementer's discretion.
- **No `depends_on`** — parallel-safe with the other two stories; no
  shared files or APIs. Sequenced last only for PR-shape cleanliness.

## Implementation notes (2026-05-23)

Applied `sort -u adjectives.txt` to produce 177 unique entries, preserving
the trailing newline. Verified:

- `wc -l adjectives.txt` → 177
- `sort adjectives.txt | uniq -c | awk '$1>1' | wc -l` → 0 (no duplicates)
- `go test ./internal/portal/playground/wordlist/...` → ok (TestPick_Diversity
  still clears the 900/1000-distinct threshold; effective adj×animal space is
  now 177 × 182 = 32 214, well above the 1 000-pick sampling).

Did not pad back up to 256 entries — the diversity test passes and the parent
feature acceptance only requires "no duplicates". Padding can land later as
a follow-up if/when wordspace breadth becomes a concern.

## Review (2026-05-23)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: `wordlist_test.go:25` comment still references "~239 adj × ~182 animals
= ~43k+ combinations" — actual joint space is now 177 × 182 ≈ 32k. The 900/1000
diversity threshold still clears comfortably, so the test logic is unaffected;
only the prose number is stale. Not worth a substrate item.

**Notes**: Pure data-only change. Verified:
- `wc -l adjectives.txt` = 177 (matches acceptance).
- `sort adjectives.txt | uniq -c | awk '$1>1' | wc -l` = 0 (no duplicates).
- File is sorted (`sort -c` passes); diff cleanly interleaves previously-duplicated
  entries into their alphabetical positions, keeping the file's existing ordering
  convention.
- `go test ./internal/portal/playground/wordlist/...` → ok.
- Trailing newline preserved; embed contract unchanged.
- Implementer's decision not to pad to 256 entries is within the design's
  explicit "optional / implementer's discretion" envelope.
