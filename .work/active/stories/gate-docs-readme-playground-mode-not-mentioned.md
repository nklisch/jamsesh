---
id: gate-docs-readme-playground-mode-not-mentioned
kind: story
stage: implementing
tags: [documentation]
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: docs
created: 2026-05-24
updated: 2026-05-24
---

# README.md says nothing about the playground feature that shipped in v0.4.0

## Drift category
readme-staleness

## Location
- Doc: `README.md` (no current mention of the playground)
- Code: `docs/VISION.md:46-48,64-67` (already documents playground as a first-class capability)

## Current doc text
> README's "What it is" section covers the durable jam model only.

## Reality
`docs/VISION.md` calls out the optional zero-friction playground mode as a first-class capability for first-contact evaluation. The README is the first thing a prospective operator reads; it should acknowledge the playground exists even if the full operator config lives in `SELF_HOST.md`.

## Required edit
Add a short paragraph (or bullet) in the "What it is" section noting the optional ephemeral-anonymous playground mode for first-contact evaluation, with a one-line pointer to `docs/SELF_HOST.md` §15 for operator config.
