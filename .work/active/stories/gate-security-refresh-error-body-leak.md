---
id: gate-security-refresh-error-body-leak
kind: story
stage: drafting
tags: [security, plugin, logging]
parent: feature-server-secret-log-hygiene
depends_on: []
release_binding: null
gate_origin: security
created: 2026-05-24
updated: 2026-05-25
---

# Refresh token error path leaks raw upstream response bodies into local stderr/log

## Severity
Low

## Domain
Error Handling & Logging

## Location
`cmd/jamsesh/portalclient/refresh.go:85-88`

## Evidence
```go
if resp.StatusCode != http.StatusOK {
    body, _ := io.ReadAll(resp.Body)
    return fmt.Errorf("portalclient: refresh returned %d: %s", resp.StatusCode, body)
}
```

The unfiltered response body propagates up through
`fmt.Errorf("portalclient: token refresh failed: %w", refreshErr)` in
`client.go:69-71` and ultimately surfaces in CLI output / hook logs. If the
portal ever includes the offending refresh token (or a hash prefix) in an
error envelope, it lands in user-visible state. The same pattern exists in
`client.go:139-140` (`GetJSON`) and `client.go:218-220`
(`PostJSON`/`PostJSONAnon`).

## Remediation direction
Bound the logged-response slice (e.g. cap at 512 bytes), strip any
`Authorization`-like fields, and prefer the typed `error`/`message` envelope
keys over the raw body.
