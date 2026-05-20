ARG BINARY=portal
# alpine:3.21 + git is the runtime base: the portal shells out to `git init
# --bare`, `git-receive-pack`, and `git-upload-pack`, so a git binary must be
# present. The portal binary itself is built CGO_ENABLED=0 (fully static),
# so musl libc is fine. ca-certificates supplies the trust roots needed for
# outbound TLS (GitHub OAuth, transactional email providers).
FROM alpine:3.21
ARG BINARY
ARG TARGETOS
ARG TARGETARCH
RUN apk add --no-cache git ca-certificates
COPY --chmod=0755 ${BINARY}-${TARGETOS}-${TARGETARCH} /usr/local/bin/portal
EXPOSE 8443
USER nobody
ENTRYPOINT ["/usr/local/bin/portal"]
