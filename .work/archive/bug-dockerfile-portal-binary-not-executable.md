---
id: bug-dockerfile-portal-binary-not-executable
kind: story
stage: done
tags: [bug, infra, docker, self-host]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-19
updated: 2026-05-19
---

# Dockerfile: portal binary lands at 0644, container fails with `exec: permission denied`

## Brief

The release workflow produces the per-arch portal binary as a plain file
(no executable bit set — mode `0644`). The image's `COPY` instruction
preserves source mode, so `/usr/local/bin/portal` inside the published
image is non-executable. On `docker run` the container immediately exits
with `exec /usr/local/bin/portal: permission denied`.

This was hit by a self-hoster during a fresh `docker compose up -d` of
the official `nklisch/jamsesh` image and worked around with a 4-line
overlay Dockerfile that re-copies + chmods the binary. The image itself
is broken as shipped; the fix belongs in this repo's `Dockerfile`.

## Strategic decisions

Resolved at scope.

- **Fix shape: `COPY --chmod=0755` on the existing line.** One-line
  change, no extra `RUN` layer. `COPY --chmod` has been in BuildKit since
  Docker 20.10 (2020) — the build is already BuildKit-based (multi-arch
  via `docker buildx`), so the flag is available.
- **Don't fix the build to ship the binary executable.** The Go build
  produces an executable, but the GitHub Actions artifact upload +
  download round-trip drops the executable bit. Reasserting mode in the
  Dockerfile is more reliable than trying to thread chmod through every
  CI step that touches the binary.
- **No entrypoint script change.** `ENTRYPOINT
  ["/usr/local/bin/portal"]` (exec form) is correct; the bug is purely
  filesystem mode.

## Acceptance criteria

- [ ] `Dockerfile` line 12 changed from
      `COPY ${BINARY}-${TARGETOS}-${TARGETARCH} /usr/local/bin/portal`
      to
      `COPY --chmod=0755 ${BINARY}-${TARGETOS}-${TARGETARCH} /usr/local/bin/portal`.
- [ ] Local smoke (single-arch is fine):
      `docker buildx build --load --platform linux/amd64 \
       --build-arg BINARY=portal -t jamsesh:smoke .` then
      `docker run --rm jamsesh:smoke --version` (or `--help`) succeeds —
      no `permission denied`.
- [ ] Verify the bit is set inside the image:
      `docker run --rm --entrypoint /bin/sh jamsesh:smoke -c 'ls -l /usr/local/bin/portal'`
      shows `-rwxr-xr-x`.
- [ ] No other change to `Dockerfile` (USER nobody, EXPOSE, ENTRYPOINT
      all untouched).

## Notes

- The `Dockerfile.router` and `Dockerfile.dev` images may have the same
  shape — Dockerfile.dev builds from source so chmod is implicit, but
  `Dockerfile.router` should be spot-checked. If it `COPY`s a prebuilt
  binary, it needs the same fix; out of scope of this story if the
  smoke check confirms it's unaffected, but worth a one-line audit.
- Re-test by pulling the next published tag in a fresh environment —
  the canonical proof is `docker run nklisch/jamsesh:vX.Y.Z` working
  with no overlay.
- The reporter's overlay Dockerfile (`/srv/jamsesh-portal/Dockerfile`)
  can be deleted once a re-published image is verified.

## Out of scope

- Auditing every artifact step in `release.yml` to find where the bit
  is dropped. The Dockerfile reassertion is the fix; CI archaeology can
  happen separately if mode-preservation matters elsewhere.
- Reproducing in CI as a regression test. Image-build smoke would be a
  nice e2e but belongs to a broader release-pipeline test story.

## Implementation notes

- Dockerfile: COPY → COPY --chmod=0755 (line 12).
- Dockerfile.router: same fix (line 11) — audit confirmed same shape, same bug.
- Smoke: Option A (real go build + docker buildx). Built portal-linux-amd64 via `GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build ./cmd/portal`, built `jamsesh:smoke` via `docker buildx build --load --platform linux/amd64 --build-arg BINARY=portal`. Verified `ls -l /usr/local/bin/portal` inside image shows `-rwxr-xr-x`. Smoke artifacts cleaned up (binary removed, image removed).
- Reporter's overlay Dockerfile at `/srv/jamsesh-portal/Dockerfile` can be deleted once the next published image is verified.

## Review (2026-05-19)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Two-line mechanical diff (`COPY` → `COPY --chmod=0755` in both
`Dockerfile` and `Dockerfile.router`). Smoke verified with a real go build
+ docker buildx — `/usr/local/bin/portal` lands at `-rwxr-xr-x` in the
image. The roll-in of `Dockerfile.router` matches the audit note in the
story's Notes section and is the right call: same shape, same bug, same
one-line fix. No tests added, correctly — the story explicitly excluded
regression-test work from scope and Dockerfile test infrastructure does
not exist in this repo. What's now possible: the next published
`nklisch/jamsesh` image will run without operator overlay; self-hosters
who hit the `exec: permission denied` reproducer can drop their custom
Dockerfile wrappers as soon as a fresh tag ships.
