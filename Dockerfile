ARG BINARY=portal
# debian:bookworm-slim provides git (needed for git init --bare, git merge-file)
# while staying small. distroless/static cannot run git since it ships no glibc
# and no external binaries. See .work/backlog/portal-docker-image-missing-git-binary.md
FROM debian:bookworm-slim
RUN apt-get update \
    && apt-get install -y --no-install-recommends git ca-certificates \
    && rm -rf /var/lib/apt/lists/*
ARG BINARY
ARG TARGETOS
ARG TARGETARCH
COPY ${BINARY}-${TARGETOS}-${TARGETARCH} /usr/local/bin/portal
EXPOSE 8443
USER nobody:nogroup
ENTRYPOINT ["/usr/local/bin/portal"]
