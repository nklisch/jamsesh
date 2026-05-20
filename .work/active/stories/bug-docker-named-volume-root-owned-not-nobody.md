---
id: bug-docker-named-volume-root-owned-not-nobody
kind: story
stage: implementing
tags: [bug, infra, docker, self-host, documentation]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-19
updated: 2026-05-19
---

# Docker named volume mounts as root, breaks SQLite open under `USER nobody`

## Brief

The portal runs as `USER nobody` (UID 65534) — see `Dockerfile:14`. The
SQLite default deploy writes to `/data/jamsesh.db` via the `jamsesh_data`
named volume. As shipped, the image does not create `/data` with
`nobody:nobody` ownership, so when Docker first materializes the named
volume the mountpoint inherits root:root and SQLite cannot open the DB.
A self-hoster hit this during a fresh `docker compose up -d` and
worked around it with a one-shot
`docker run --rm -v jamsesh_data:/data alpine chown -R 65534:65534 /data`.

Compounding the bug, `deploy/compose/README.md:79-81` claims:

> Docker named volumes inherit ownership from the first container write
> — they work out of the box.

That claim is wrong as stated. What Docker actually does on first
volume materialization is copy the contents and ownership *of the image
mountpoint directory* into the volume. Because `/data` does not exist
in our image (no `mkdir`, no `chown`), Docker creates it root-owned and
the "work out of the box" promise breaks.

Two coordinated fixes: pre-create `/data` with the right ownership in
the image so named volumes inherit nobody:nobody, and correct the
compose README wording to describe what Docker actually does.

## Strategic decisions

Resolved at scope.

- **Fix the image, not just the docs.** The reporter's workaround (an
  external chown) is operational toil that should not be required for
  the default SQLite deploy. Pre-creating `/data` with nobody ownership
  in the image makes named volumes work without operator action — which
  is the experience the README already promises.
- **Pre-create via `RUN mkdir -p /data && chown nobody:nobody /data`,
  placed BEFORE `USER nobody`.** The chown needs root; `USER nobody`
  drops privileges, so the mkdir+chown must come earlier in the
  Dockerfile. No need for an entrypoint script or tini — a build-time
  filesystem fix is sufficient because Docker copies image-mountpoint
  ownership to a fresh named volume.
- **No entrypoint chown.** Adding a privileged entrypoint that chowns
  /data at every container start would defeat `USER nobody` and is
  unnecessary once the image mountpoint is correct.
- **Host-bind-mount path stays manual.** The existing README section
  about chowning a host directory (lines 81-85) is correct and stays.
  Named volumes are the default path and the one being fixed; host
  binds remain operator-managed.
- **README wording: match Docker's actual behavior.** Replace the
  "first container write" sentence with the correct mechanism — that
  Docker copies the image mountpoint's ownership into a fresh named
  volume, and our image pre-creates `/data` with nobody ownership so
  this works without intervention.

## Acceptance criteria

- [ ] `Dockerfile`: insert `RUN mkdir -p /data && chown nobody:nobody /data`
      after the `apk add` line and before `USER nobody`.
- [ ] Confirm in image inspect:
      `docker run --rm --entrypoint /bin/sh jamsesh:smoke -c 'ls -ld /data'`
      shows `drwxr-xr-x ... nobody nobody`.
- [ ] Fresh-volume smoke:
      `docker volume rm jamsesh_smoke_data || true && \
       docker run --rm -v jamsesh_smoke_data:/data --entrypoint /bin/sh \
         jamsesh:smoke -c 'ls -ld /data && touch /data/probe && ls -l /data/probe'`
      — both the dir and the probe file owned by nobody, no permission errors.
- [ ] `deploy/compose/README.md` lines ~78-81 rewritten to describe
      what Docker actually does: named volumes inherit ownership from
      the image mountpoint, and this image pre-creates `/data` with
      nobody ownership so SQLite can write to the default
      `jamsesh_data` volume without operator action. Host bind-mount
      section (chown -R 65534:65534) stays as-is.
- [ ] End-to-end: `cd deploy/compose && cp .env.example .env && \
       (set JAMSESH_DOMAIN to a placeholder) && docker compose up -d`
      then `docker compose logs portal` shows the portal opening
      `/data/jamsesh.db` without permission errors. (TLS provisioning
      will fail without a real domain, but the SQLite open path must
      succeed — that's the bug under test.)

## Notes

- This builds on the sibling story
  `bug-dockerfile-portal-binary-not-executable` only insofar as both
  touch the same `Dockerfile`. There is no ordering dependency: the
  two RUN/COPY edits are independent and either can land first. Kept
  as peer stories so they can be picked up in either order; combine
  in the same commit if implemented together.
- UID 65534 (`nobody`) is consistent across alpine's `passwd` and the
  reporter's observed UID. No need to parametrize.
- The reporter's one-shot `chown -R 65534:65534 /data` workaround
  remains valid for existing deploys that already created the volume
  before this fix lands. Worth a one-line "Existing deploys" note in
  the README's troubleshooting section if convenient — optional, not
  blocking.
- Re-test the fix by destroying any local `jamsesh_smoke_data` volume
  between runs; a previously-fixed volume keeps working even with the
  bug, masking the regression.

## Out of scope

- Switching off `nobody` to a named app user. UID 65534 is fine; the
  fix is about the volume mountpoint, not the user identity.
- Migrating to a non-root entrypoint pattern (tini + gosu). The
  build-time fix is sufficient for the named-volume path.
- A docker-compose-level `command` or init container to chown at
  runtime. Not needed once the image mountpoint is correct.
