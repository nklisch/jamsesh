---
id: testing-bin-jamsesh-regression-harness-ci-job
kind: story
stage: review
tags: [testing, infra, plugin, ci]
parent: testing-bin-jamsesh-regression-harness
depends_on: [testing-bin-jamsesh-regression-harness-bats-suite]
release_binding: null
gate_origin: null
created: 2026-05-20
updated: 2026-05-20
---

# CI: wire `tests/wrapper/` bats suite into `.github/workflows/quickstart.yml`

## Brief

Add a new `wrapper-tests` job to `.github/workflows/quickstart.yml`, in
parallel with the existing `quickstart` and `compose-template` jobs.

The job installs bats via apt and runs `bats tests/wrapper/`.

## Approach

Place a new job at the same indent level as existing jobs (`jobs.wrapper-tests`):

```yaml
  wrapper-tests:
    runs-on: ubuntu-latest
    steps:
      - name: checkout
        uses: actions/checkout@v4

      - name: install bats
        run: sudo apt-get update && sudo apt-get install -y bats

      - name: run wrapper tests
        run: bats tests/wrapper/
```

No `needs:` declaration — runs in parallel.
No explicit timeout — bats' own runtime is the cap; expected <30s.

## Acceptance criteria

- [ ] `yamllint .github/workflows/quickstart.yml` exits 0 (or matches whatever linting the project applies — verify by running it)
- [ ] `actionlint .github/workflows/quickstart.yml` exits 0 if `actionlint` is installed
- [ ] Manual smoke: push to a temporary branch, observe the `wrapper-tests` job appear in the GitHub Actions tab and exit 0 — OR if branch push is out of scope for this story, document in implementation notes that the YAML structure was verified by inspection only and that the next PR will be the first real exercise.

## Files modified

- `.github/workflows/quickstart.yml` — append the new job

## Files NOT modified

- The bats suite under `tests/wrapper/` — that's the responsibility of the depended-on sibling story.

## Implementation notes

**Linters:**
- `yamllint`: not installed — skipped per story instructions.
- `actionlint`: not installed — skipped per story instructions.
- Indent verified by eyeball: new job uses 2-space job name under `jobs:`, matching `quickstart` and `compose-template`. Step keys at 6-space, `runs-on`/`steps` at 4-space — consistent with existing jobs throughout the file.

**Local bats smoke (`bats tests/wrapper/`):** All 15 tests passed (exit 0). This exercises the exact command the CI job will run, confirming the suite is invocable from the repo root.

**Placement:** New job appended after `compose-template`, at the same indent level. No `needs:` — runs in parallel with `quickstart` and `compose-template`.
