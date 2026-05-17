---
id: portal-docker-image-missing-git-binary
kind: story
stage: implementing
tags: [bug, infra, docker]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Portal Docker image: missing `git` binary causes session creation to fail

## Bug

`POST /api/orgs/{orgID}/sessions` returns 500 with:

```
sessions: create repo: storage: git init --bare: exec: "git": executable file not found in $PATH
```

## Root cause

`internal/portal/storage/repo.go` calls `exec.CommandContext(ctx, "git", "init", "--bare", p)`
and `internal/portal/automerger/merge.go` calls `exec.Command("git", "merge-file", ...)`.

The production Dockerfile (`Dockerfile`) uses `gcr.io/distroless/static:nonroot` as
its base image. Distroless-static contains no shell, no standard utilities, and no
`git` binary. Any attempt to create a session (which initialises a bare repo) fails
immediately with a 500.

Discovered during e2e test implementation for `epic-e2e-tests-golden-path-session-lifecycle`.

## Fix

Switch the Dockerfile base image to one that includes `git`. Options in order of
preference:

1. **Multi-stage build**: keep `distroless/static` for the Go binary but add a
   `git` binary layer from a Debian/Alpine stage:
   ```dockerfile
   FROM debian:bookworm-slim AS gitbin
   RUN apt-get update && apt-get install -y --no-install-recommends git && rm -rf /var/lib/apt/lists/*

   FROM gcr.io/distroless/static:nonroot
   COPY --from=gitbin /usr/bin/git /usr/bin/git
   # git also needs shared libs — distroless/static has none. Use base instead:
   ```
   This is complex because `git` links against glibc which distroless-static lacks.

2. **Switch to `gcr.io/distroless/base-debian12:nonroot`** (has glibc) and copy
   `git` from a Debian stage:
   ```dockerfile
   FROM debian:bookworm-slim AS gitbin
   RUN apt-get update && apt-get install -y --no-install-recommends git && rm -rf /var/lib/apt/lists/*

   FROM gcr.io/distroless/base-debian12:nonroot
   COPY --from=gitbin /usr/bin/git /usr/bin/git
   COPY --from=gitbin /usr/lib/git-core /usr/lib/git-core
   # Copy required shared libs...
   ```
   This is still fragile.

3. **Use `debian:bookworm-slim` directly** and install git:
   ```dockerfile
   FROM debian:bookworm-slim
   RUN apt-get update && apt-get install -y --no-install-recommends git ca-certificates && \
       rm -rf /var/lib/apt/lists/*
   ARG BINARY
   COPY ${BINARY}-linux-amd64 /usr/local/bin/portal
   EXPOSE 8443
   USER nobody:nogroup
   ENTRYPOINT ["/usr/local/bin/portal"]
   ```
   Slightly larger image but straightforward. Recommended for now.

## Acceptance criteria

- [ ] `docker build -t jamsesh/portal:e2e .` produces an image where `git --version` works
- [ ] `POST /api/orgs/{orgID}/sessions` succeeds inside the e2e Testcontainers stack
- [ ] `TestSessionLifecycleJoinAndPush` passes end-to-end

## Workaround

While this bug is open, `TestSessionLifecycleJoinAndPush` skips with:

```
t.Skip("portal image missing git — see portal-docker-image-missing-git-binary")
```

Remove the skip once the image is fixed.
