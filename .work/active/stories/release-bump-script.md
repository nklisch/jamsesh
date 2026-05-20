---
id: release-bump-script
kind: story
stage: review
tags: [infra, tooling]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-18
updated: 2026-05-19
---

# Release bump script — one command to version-bump, tag, push

## Brief

A simplified bump script that collapses the multi-file version bump + tag +
push portion of the release flow into a single command. Replaces the three
manual `sed` invocations currently documented as separate steps in
`docs/RELEASING.md` "Cutting a release".

Today the maintainer has to: edit `bin/jamsesh`'s `JAMSESH_PLUGIN_VERSION`,
edit `deploy/compose/.env.example`'s `JAMSESH_VERSION`, edit
`.claude-plugin/plugin.json`'s `version`, commit, tag, push. Six manual
steps that have to be done in the right order with the right strings;
easy to forget one.

After this story: `scripts/release-bump.sh v0.X.Y` does all of it, with
pre-flight validation. The script becomes the canonical "ship phase" tool;
`release-deploy`'s ship phase (and the operator running the release
manually) both call it.

## Strategic decisions

Resolved at scope.

- **Language: bash.** Consistent with `bin/jamsesh` precedent. Same
  portability target — Linux + macOS + Git Bash on Windows.
- **Location: `scripts/release-bump.sh`.** New top-level `scripts/`
  directory; release/tooling helpers belong there. `Makefile` targets
  can wrap it later if useful.
- **What it bumps (three files)**:
  1. `bin/jamsesh` — `JAMSESH_PLUGIN_VERSION` constant (uses `v` prefix).
     Sed pattern: `s/^readonly JAMSESH_PLUGIN_VERSION=.*/readonly JAMSESH_PLUGIN_VERSION="<vX.Y.Z>"/`.
  2. `deploy/compose/.env.example` — `JAMSESH_VERSION` (uses `v` prefix).
     Sed pattern: `s/^JAMSESH_VERSION=.*/JAMSESH_VERSION=<vX.Y.Z>/`.
  3. `.claude-plugin/plugin.json` — `version` field (uses BARE
     `X.Y.Z`, no `v` prefix). Updated via `jq`.
- **Pushes: yes, branch + tag.** User explicitly said "and pushes".
  The tag push triggers `release.yml`. Branch push first, then tag push
  (so the tag's commit is reachable from main when the workflow fires).
- **Annotated tag, no GPG signing.** Keyless cosign in `release.yml`
  is the supply-chain trust anchor; GPG signing is redundant unless
  the user wants belt-and-suspenders. Easy to add later.
- **Pre-flight validation, fail-fast:**
  - Argument matches `^v[0-9]+\.[0-9]+\.[0-9]+$` (semver with `v`).
  - Working tree is clean (`git diff --quiet`, `git diff --cached --quiet`).
  - Currently on `main` (or `git branch --show-current` matches an allowed
    list — main only for v1; can relax later).
  - The tag doesn't already exist locally or on origin.
  - `bin/jamsesh`, `deploy/compose/.env.example`, `.claude-plugin/plugin.json`
    all exist and have the expected version line.
- **`--dry-run` flag.** Print what would change without doing it. Useful
  for the maintainer to spot-check before triggering CI.

## Acceptance criteria

- [ ] `scripts/release-bump.sh` exists, executable, starts with
      `#!/usr/bin/env bash` and `set -euo pipefail`.
- [ ] `scripts/release-bump.sh v0.1.1` (with a clean working tree, on
      main, with tag not yet existing): edits the three files in place;
      stages them with explicit paths (no `-A`); creates a single
      commit `release-prep: v0.1.1`; creates an annotated tag
      `v0.1.1`; pushes `main` then `v0.1.1` to `origin`.
- [ ] `scripts/release-bump.sh --dry-run v0.1.1`: prints what would
      change to stdout/stderr; touches nothing; exits 0.
- [ ] Invalid arg (`v1`, `1.2.3`, `latest`, empty): clear error message,
      exit non-zero, nothing changed.
- [ ] Dirty working tree: clear error, refuses to run.
- [ ] Wrong branch: clear error, refuses to run.
- [ ] Tag already exists (local or origin): clear error, refuses to run.
- [ ] Idempotence pre-check: if any of the three files already shows the
      target version, the script SHOULD STILL proceed (the maintainer may
      have manually bumped one file and not the others) — but log a
      warning so they notice. **Open**: alternative is to refuse if all
      three are already at target. Decide at implementation: log+continue
      is gentler.
- [ ] `docs/RELEASING.md` "Cutting a release" steps 2 + 3 (the manual
      compose-template and bin/jamsesh sed bumps) collapsed into a single
      step pointing at `scripts/release-bump.sh`. The manual recipes can
      stay as a fallback under "Manual recipe (if the script breaks)" or
      be deleted entirely — operator-friendlier to keep the fallback.
- [ ] Local smoke: run `scripts/release-bump.sh --dry-run v9.9.9` from a
      clean tree; spot-check the diff output.

## Notes

- Don't include the CHANGELOG update in this script — that's handled by
  `release-deploy`'s phase 5.5 BEFORE this script runs. The flow is:
  `/agile-workflow:release-deploy v0.1.1` (binds items, runs gates,
  drafts + commits CHANGELOG, walks to readiness halt) → maintainer
  reviews + approves → `scripts/release-bump.sh v0.1.1` (this script:
  bumps versions, tags, pushes) → `release.yml` fires.
- The script can later be invoked directly from
  `release-deploy`'s ship phase (workflow Phase 6 in the skill), making
  the whole release flow `/agile-workflow:release-deploy v0.1.1` end to
  end. Not part of this story — out of scope; substrate-skill
  modification belongs to a separate item if desired.
- The `bin/jamsesh` and `.env.example` lines are anchored at the start
  of the line (`^readonly` and `^JAMSESH_VERSION=`); the sed patterns
  must use those anchors so commented-out copies further down don't
  accidentally match.
- `.claude-plugin/plugin.json` is JSON; use `jq` (already required for
  the existing release.yml marketplace job, but that's deleted now).
  Check `command -v jq >/dev/null` and `die` cleanly if missing.

## Out of scope

- GPG-signed tags. Maintainer can opt in by editing the script's `git tag`
  invocation; documented in the script's header comment.
- Updating `CHANGELOG.md` (handled by `release-deploy` phase 5.5).
- Running quality gates (handled by `release-deploy` phases 4).
- Triggering or watching `release.yml` after push (the maintainer's job
  via `gh run watch` per `docs/RELEASING.md`).

## Implementation notes

- Script structure mirrors `bin/jamsesh`: `set -euo pipefail`, `die()`/`warn()`/`info()`
  helpers, leading section comments. Added a `run()` helper that either executes or
  echoes commands in dry-run mode — cleaner than threading `if DRY_RUN` guards
  through every action.
- Sed edits use a portable tempfile pattern (`sed ... file > file.tmp && mv`) rather
  than `sed -i`, which avoids the GNU vs BSD `-i` suffix difference on macOS.
- One non-obvious fix: `jq` by default normalises `—` to the literal em-dash
  character (`—`). Without `--ascii-output`, every run would produce a cosmetically
  different `plugin.json` that git would flag as modified. Added `--ascii-output` to
  both the apply step and the dry-run diff step so the output is byte-for-byte
  identical to the source file when the only change is `.version`.
- Dirty-tree check fires before idempotence warnings — correct ordering (no point
  warning about already-bumped files if we're going to refuse anyway).
- `--dry-run v0.1.1` correctly hits the "tag already exists locally" pre-flight
  rather than the idempotence path, because v0.1.1 is the current shipped tag. The
  idempotence path activates for future versions that were manually half-bumped.
- `shellcheck` not available on this machine — skipped. The script uses only
  standard constructs (`[[`, `$()`, `"${VAR:-}"`) and was hand-reviewed for
  quoting and expansion safety.
- Dry-run smoke (`--dry-run v9.9.9`) output highlights:
  - `bin/jamsesh`: `-readonly JAMSESH_PLUGIN_VERSION="v0.1.1"` / `+readonly JAMSESH_PLUGIN_VERSION="v9.9.9"`
  - `deploy/compose/.env.example`: `-JAMSESH_VERSION=v0.1.1` / `+JAMSESH_VERSION=v9.9.9`
  - `.claude-plugin/plugin.json`: `-"version": "0.1.1"` / `+"version": "9.9.9"`, `—` preserved
  - All five `[dry-run]` git commands printed; exit 0.
  - `git status` after: only `?? scripts/` (untracked new dir) — nothing modified.
- `docs/RELEASING.md`: old steps 2 + 3 (two separate manual sed recipes) replaced by
  a single step 2 invoking `scripts/release-bump.sh vX.Y.Z`, with the original sed
  recipes preserved verbatim under "Manual recipe (if the script breaks)". Old steps
  5–7 renumbered to 3–5 (step 5 "push the tag" is now inside the script, so removed).
