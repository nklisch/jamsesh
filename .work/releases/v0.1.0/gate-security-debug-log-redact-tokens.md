---
id: gate-security-debug-log-redact-tokens
kind: story
stage: done
tags: [security, portal, documentation]
parent: null
depends_on: []
release_binding: v0.1.0
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

## Implementation notes

### Files changed

- `internal/portal/logging/redact.go` — new file; exports `RedactQueryTokens(s string) string`. Parses the raw query string pair-by-pair preserving percent-encoding on non-sensitive values; replaces sensitive param values with the literal `<redacted>`. Falls back to a compiled regex (`sensitiveParamRE`) when `url.ParseQuery` rejects the input so a raw token can never pass through.
- `internal/portal/logging/logging.go` — added `"query", RedactQueryTokens(r.URL.RawQuery)` field to the `slog.InfoContext` call in `Access`. Path is unchanged (still `r.URL.Path`, which excludes the query string by definition); the new `query` field makes the redacted query string visible and keeps the log shape consistent even when there are no params (emits `""`).
- `internal/portal/logging/logging_test.go` — added `TestRedactQueryTokens` (table-driven, 9 sub-cases), `TestRedactQueryTokensMultipleValues`, `TestRedactQueryTokensMalformed`, and `TestAccessMiddlewareLogsRedactedQuery` (integration: verifies the middleware emits a `query` field with `<redacted>` and that the raw token is absent from the log line).
- `docs/SELF_HOST.md` — added "Access-log query-string redaction" subsection under §8 Monitoring / Log output describing the redaction behaviour and noting that operators should still avoid `JAMSESH_LOG_LEVEL=-4` in production because third-party middleware is not covered.

### Redacted param set

`token`, `code`, `state`, `ticket` (case-insensitive match). Covers magic-link tokens, OAuth authorization codes, OAuth CSRF state, and ticket-based auth flows.

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none.

**Notes**: Implementation is precise — pair-by-pair walk avoids re-encoding
non-sensitive values (a common pitfall with naive `url.ParseQuery` + re-Encode).
Fallback regex on parse-failure guarantees the function never returns a raw
token, even for malformed inputs. Both URL-shaped and raw-query inputs handled.
Param set (`token`, `code`, `state`, `ticket`) is the right conservative cover
for the project's auth flows (and is consistent with the newly-shipped ws-ticket
flow). Doc note correctly frames this as defense-in-depth rather than a license
to enable DEBUG in production.
