---
id: gate-tests-wordlist-diversity-threshold-and-length-band
kind: story
stage: drafting
tags: [testing, portal, playground]
parent: null
depends_on: []
release_binding: null
gate_origin: tests
created: 2026-05-24
updated: 2026-05-24
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
