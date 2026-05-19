---
id: feature-docker-compose-self-host-template-docs
kind: story
stage: done
tags: [infra, documentation]
parent: feature-docker-compose-self-host-template
depends_on: [feature-docker-compose-self-host-template-template-files]
release_binding: null
gate_origin: null
created: 2026-05-18
updated: 2026-05-18
---

# Compose template — documentation updates

## Scope

Wire the new `deploy/compose/` template into existing operator-facing docs
and the release checklist:

- `docs/SELF_HOST.md` — add §1.0 "Quickstart with Docker Compose" at the
  top of §1 ("Install"), pointing at the template.
- `README.md` — replace the "Operator quickstart" section to lead with the
  compose template path, retaining the bare `docker run` snippet as a
  secondary option for purely-local trials.
- `docs/RELEASING.md` — add a "Bump the compose template" step in
  "Cutting a release" between current step 1 (drain the queue) and step 2
  (confirm CHANGELOG). Renumber subsequent steps.

## Implementation

Use the exact deltas specified in the parent feature's "Unit 2: Documentation
updates" section. Key points:

- Cross-references use relative paths consistent with the rest of the
  repo (`../deploy/compose/README.md` from `docs/`, `deploy/compose/README.md`
  from root README, `./SELF_HOST.md` etc.).
- The existing SELF_HOST §1 ("Docker (recommended)" / "Binary" / "systemd
  unit") stays unchanged below the new §1.0 — this is an additive insert,
  not a rewrite.
- The RELEASING.md step includes a one-line `sed -i` command for the
  version bump so it's copy-pasteable.

## Acceptance Criteria

- [ ] `docs/SELF_HOST.md` opens §1 with a new §1.0 "Quickstart with
      Docker Compose" subsection that references `deploy/compose/`.
- [ ] `README.md` "Operator quickstart" leads with the 4-step compose
      template flow, keeps the `docker run` snippet as a "kick the
      tires locally" secondary option.
- [ ] `docs/RELEASING.md` has a numbered "Bump the compose template's
      `JAMSESH_VERSION` pin" step between drain-the-queue and
      confirm-CHANGELOG, with a copy-pasteable `sed` command.
- [ ] Subsequent steps in `docs/RELEASING.md` renumbered consistently.
- [ ] All relative-path cross-references resolve (manually verify each
      link from the file's location).
- [ ] No existing operator-facing content removed — this story is
      additive + a single replacement in README's quickstart block.

## Implementation Notes

### Files edited

- **`docs/SELF_HOST.md`** — Inserted new §1.0 "Quickstart with Docker Compose"
  subsection between the `## 1. Install` heading (line 10) and the existing
  `### Docker (recommended)` subsection (originally line 12, now line 32 after
  insert). Additive; no existing content removed.

- **`README.md`** — Replaced the "Operator quickstart" section body (lines 49–67
  in the original). The bare `docker run` snippet is retained as a secondary
  "kick the tires locally" option after the compose template flow. The paragraph
  about TLS/OAuth/database pointing at SELF_HOST.md is preserved in updated form.

- **`docs/RELEASING.md`** — Inserted new step 2 ("Bump the compose template's
  `JAMSESH_VERSION` pin") in the "Cutting a release" section between step 1
  (Drain the queue, line 39) and old step 2 (Confirm the CHANGELOG). Renumbered
  subsequent steps: old 2→3, old 3→4, old 4→5, old 5→6. Final sequence in
  "Cutting a release": 1–6 with no gaps or duplicates.

### Cross-reference paths verified

- From `docs/SELF_HOST.md`: `../deploy/compose/README.md` resolves correctly
  (docs/ → repo root → deploy/compose/).
- From `README.md` (repo root): `deploy/compose/README.md` and
  `docs/SELF_HOST.md` both resolve correctly.
- `deploy/compose/.env.example` referenced in RELEASING.md exists on disk.
