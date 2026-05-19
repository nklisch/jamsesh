---
id: feature-cc-plugin-wrapper-binary-fetch-docs
kind: story
stage: review
tags: [infra, plugin, documentation]
parent: feature-cc-plugin-wrapper-binary-fetch
depends_on: [feature-cc-plugin-wrapper-binary-fetch-script]
release_binding: null
gate_origin: null
created: 2026-05-18
updated: 2026-05-18
---

# Docs — align RELEASING, SECURITY, and (optionally) README

## Scope

Bring foundation docs and release-checklist in line with the wrapper-script
distribution model. Three files:

- `docs/RELEASING.md` — remove marketplace bootstrap entirely; add the
  bin/jamsesh version-bump step alongside the existing compose-template bump;
  retune the Overview to describe the wrapper rather than the mirror repo.
- `docs/SECURITY.md` — adjust supply-chain wording in §"Supply chain and
  integrity" to describe GH-release distribution and wrapper-time verification.
- `README.md` (optional) — add a short "Install the Claude Code plugin"
  section if it doesn't exist, pointing at `nklisch/jamsesh`.

## Implementation

Use the exact deltas from the parent feature's Unit 3 spec. Key points:

- **RELEASING.md Overview step 8** rewritten to describe the wrapper.
- **RELEASING.md "Cutting a release"**: add a new step (just after the
  compose-template bump) for the wrapper version bump. Renumber.
- **RELEASING.md "One-time bootstrap: marketplace plugin repo"**: delete
  the entire section (currently ~80 lines).
- **SECURITY.md §"Supply chain and integrity"** lines 199–214: update
  the two sentences about how the binary is distributed and how
  signatures are verified.
- **README.md**: add a "Install the Claude Code plugin" section between
  "Operator quickstart" and "License" with the CC marketplace install
  flow. If the exact slash-command for installing from a non-default
  marketplace source is uncertain, prefer accuracy over speed —
  put a TODO with a backlog item rather than guessing.

Do NOT add migration-style prose. Rolling-foundation: describe NOW.

## Acceptance Criteria

- [ ] `grep -rn 'jamsesh-cc-plugin\|MARKETPLACE_DEPLOY_KEY\|marketplace repo' docs/`
      returns empty after the edit.
- [ ] `docs/RELEASING.md` "Cutting a release" steps are sequentially numbered
      with both compose-template bump AND wrapper bump present.
- [ ] `docs/RELEASING.md` no longer has the "One-time bootstrap" H2 or its body.
- [ ] `docs/SECURITY.md` §"Supply chain and integrity" describes
      GH-release-asset distribution and `bin/jamsesh` wrapper verification.
- [ ] `README.md` either has a verified plugin-install section OR a
      clearly-marked TODO with a backlog item id (don't ship wrong commands).
- [ ] All relative-path cross-references resolve.

## Notes

- The CC marketplace install command shape: per research notes, the
  marketplace config takes `source: { source: "github", repo: "owner/repo" }`.
  User-facing slash commands likely include `/marketplace add` and
  `/plugins install`. Verify against current CC docs OR ship the README
  addition with a "verify exact commands" follow-up backlog item.
- The compose-template feature already added its own RELEASING.md step
  (`feature-docker-compose-self-host-template-docs`); the wrapper bump
  is a sibling step — don't replace, add.
- The SECURITY.md tweak is small (≤6 lines) but matters for
  rolling-foundation honesty — operators reading SECURITY.md should see
  the actual distribution path.

## Implementation Notes

### Files edited

**`docs/RELEASING.md`**
- Overview step 8 (was lines 25–27): replaced marketplace-repo description
  with wrapper-script fetch model text (anchor: "Plugin install: users install
  from `nklisch/jamsesh` directly").
- "Cutting a release" section: inserted new step 3 ("Bump `bin/jamsesh`
  `JAMSESH_PLUGIN_VERSION`") as a sibling to the compose-template bump (step
  2); renumbered old steps 3–6 to 4–7. Final list: 7 sequential steps, 1–7.
- Deleted entire §"One-time bootstrap: marketplace plugin repo" including its
  leading `---` separator (was lines 95–186 including the section separator
  before "Verifying release signatures"). Zero trace of `jamsesh-cc-plugin`,
  `MARKETPLACE_DEPLOY_KEY`, or `marketplace repo` remains.

**`docs/SECURITY.md`**
- §"Supply chain and integrity" lines 201–208 (original numbering): replaced
  two sentences.
  - Sentence 1 anchor: "distributed via the marketplace repo" → "distributed
    as GitHub release assets"; added wrapper sha256 + cosign sentence.
  - Sentence 2 anchor: "verified at install time by both the marketplace and
    the self-host install flows" → "verified at fetch time by the plugin
    wrapper (`bin/jamsesh`) and at install time by the self-host install flows".

### README addition

Deferred to backlog item `docs-readme-cc-plugin-install-instructions`. The
exact user-facing Claude Code slash commands (`/marketplace add`,
`/plugins install`) need verification against current CC before publishing.
The marketplace JSON source shape (`{ "source": "github", "repo":
"nklisch/jamsesh" }`) is known; only the CLI command names need confirmation.
The backlog item contains everything the next implementer needs to dive in.

### Verification check outcomes

1. `grep -rn 'jamsesh-cc-plugin\|MARKETPLACE_DEPLOY_KEY\|marketplace repo' docs/`
   → empty output. PASS.

2. Sequential numbering check on "Cutting a release":
   `1. 2. 3. 4. 5. 6. 7.` — sequential, no gaps, no duplicates. PASS.

3. `grep -n 'GitHub release assets\|bin/jamsesh' docs/SECURITY.md`
   → 3 hits (lines 202, 203, 208). PASS.

4. `grep -n '^## One-time bootstrap' docs/RELEASING.md`
   → empty output. PASS.
