---
id: feature-docker-compose-self-host-template-docs
kind: story
stage: implementing
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
