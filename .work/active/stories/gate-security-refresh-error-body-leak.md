---
id: gate-security-refresh-error-body-leak
kind: story
stage: implementing
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
`cmd/jamsesh/portalclient/client.go:139-140` (GetJSON)
`cmd/jamsesh/portalclient/client.go:176` (GetJSONWithBearer)
`cmd/jamsesh/portalclient/client.go:218-220` (PostJSONAnon)
`cmd/jamsesh/portalclient/client.go:253-254` (PostJSON)

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

## Implementation

Add a package-private `truncatedBody` helper in `client.go` and apply it at
all five call sites.

**`cmd/jamsesh/portalclient/client.go`** — add near top of file (after imports):

```go
// maxErrBodyBytes is the maximum number of bytes read from a non-2xx response
// body when constructing error messages. Limits exposure of sensitive server
// responses in logs and CLI output.
const maxErrBodyBytes = 512

// truncatedBody reads at most maxErrBodyBytes from r and returns the result
// as a string. Does NOT close r — the caller's deferred resp.Body.Close()
// handles that.
func truncatedBody(r io.Reader) string {
    b, _ := io.ReadAll(io.LimitReader(r, maxErrBodyBytes))
    return string(b)
}
```

**Apply at each error-path site** (replace `io.ReadAll(resp.Body)` with
`truncatedBody(resp.Body)`):

1. `refresh.go:86`:
   ```go
   // Before:
   body, _ := io.ReadAll(resp.Body)
   return fmt.Errorf("portalclient: refresh returned %d: %s", resp.StatusCode, body)
   // After:
   return fmt.Errorf("portalclient: refresh returned %d: %s", resp.StatusCode, truncatedBody(resp.Body))
   ```

2. `client.go` GetJSON non-2xx:
   ```go
   return zero, fmt.Errorf("portalclient: GET %q returned %d: %s", path, resp.StatusCode, truncatedBody(resp.Body))
   ```

3. `client.go` GetJSONWithBearer non-2xx:
   ```go
   return zero, fmt.Errorf("portalclient: GET %q returned %d: %s", path, resp.StatusCode, truncatedBody(resp.Body))
   ```

4. `client.go` PostJSONAnon non-2xx:
   ```go
   return zero, fmt.Errorf("portalclient: POST %q returned %d: %s", path, resp.StatusCode, truncatedBody(resp.Body))
   ```

5. `client.go` PostJSON non-2xx:
   ```go
   return zero, fmt.Errorf("portalclient: POST %q returned %d: %s", path, resp.StatusCode, truncatedBody(resp.Body))
   ```

## Acceptance Criteria
- [ ] `truncatedBody` helper added to `client.go` (package-private)
- [ ] All five `io.ReadAll(resp.Body)` error-path reads replaced with `truncatedBody(resp.Body)`
- [ ] `TestRefresher_Refresh_ServerError` still passes
- [ ] New test `TestRefresher_Refresh_LargeErrorBodyTruncated`: server returns 401 with 1 KB body; resulting error string's body portion is ≤ 512 bytes
- [ ] New test `TestGetJSON_OversizedErrorBodyTruncated`: server returns 400 with 1 KB body; resulting error string's body portion is ≤ 512 bytes
