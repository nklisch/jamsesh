ARG BINARY=portal
FROM gcr.io/distroless/static:nonroot
ARG BINARY
ARG TARGETOS
ARG TARGETARCH
COPY ${BINARY}-${TARGETOS}-${TARGETARCH} /usr/local/bin/portal
EXPOSE 8443
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/portal"]
