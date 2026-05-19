---
id: feature-docker-compose-self-host-template-ci-smoke
kind: story
stage: implementing
tags: [infra, testing]
parent: feature-docker-compose-self-host-template
depends_on: [feature-docker-compose-self-host-template-template-files]
release_binding: null
gate_origin: null
created: 2026-05-18
updated: 2026-05-18
---

# Compose template — CI parse validation

## Scope

Add a `compose-template` job to `.github/workflows/quickstart.yml` that
validates the new compose template parses cleanly under both the default
and `postgres` profile shapes. Catches template rot from typos or
unresolved env interpolation introduced by refactors elsewhere in the
repo.

End-to-end "compose up + curl healthz" smoke is **deferred** to a follow-up
backlog item that lands once a pull-able image tag exists for the
commit under test (current PR CI cannot pull `ghcr.io/<owner>/jamsesh:vX`
for unmerged commits). The parse-only check is the v1 deliverable.

## Implementation

Use the exact job shape specified in the parent feature's "Unit 3: CI
parse-validation" section. Key points:

- New job `compose-template`, parallel to existing `quickstart` job in
  `.github/workflows/quickstart.yml`. Same triggers (`pull_request` to
  `main` + `push` to `main`).
- Three steps inside the job:
  1. Copy `.env.example` to `.env`.
  2. `docker compose config` against default profile.
  3. `docker compose --profile postgres config` against postgres profile.
  4. Stderr assertion: fail if `docker compose config` emits "variable
     is not set" warnings — `.env.example` must satisfy every
     interpolation in the default shape.
- Uses Docker Compose v2 (already on `ubuntu-latest`).
- No image pulls, no container start. Job should run in <30s.

## Acceptance Criteria

- [ ] `.github/workflows/quickstart.yml` has a new `compose-template` job.
- [ ] Job validates default profile with `docker compose config`.
- [ ] Job validates postgres profile with `docker compose --profile postgres config`.
- [ ] Job fails when `.env.example` is missing a variable that the
      compose file interpolates in the default shape (test by removing
      a var locally and confirming the job fails).
- [ ] Job runs on every PR and on `main` push, in parallel with the
      existing `quickstart` job.
- [ ] No new external action dependencies (uses bundled `docker compose`
      on the runner).

## Notes

- After this lands, file a backlog item `e2e-compose-template-up-smoke`
  to add the full "compose up + curl healthz" smoke once we have a way
  to use a locally-built image as the portal image in CI. Candidate
  approach: a `compose.ci.override.yml` that swaps the image for a
  `build:` block targeting `Dockerfile` with a locally-built portal
  binary.
