---
id: gate-security-debug-log-redact-tokens
kind: story
stage: implementing
tags: [security, portal, documentation]
parent: null
depends_on: []
release_binding: null
gate_origin: security
created: 2026-05-18
updated: 2026-05-18
---

# Token raw value passes through `slog.Info` access log path field on /auth/magic-link

## Severity
Low

## Domain
Error Handling & Logging

## Location
`internal/portal/logging/logging.go:62`

## Evidence
```go
slog.InfoContext(r.Context(), "http access",
    "method", r.Method,
    "path", r.URL.Path,
    ...
```

`r.URL.Path` excludes the query string (so `?token=…` is not logged
here), but the SPA route `/auth/magic-link` is mounted under the
catch-all SPA (`router.go:135-137`) and falls through to
`assets.Handler` which serves `index.html`. Standalone this is fine —
but when the operator runs `JAMSESH_LOG_LEVEL=-4` debug, requests under
arbitrary paths get logged. There is no allow/deny list for sensitive
paths.

## Remediation direction
Document that operators should NOT raise log level to DEBUG in
production; consider a redaction pass that strips `?token=`, `?code=`,
`?state=` from any field that is logged.
