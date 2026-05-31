# Session-Scoped Portal Client

CLI portal clients that act on a known session set
`portalclient.Client.SessionID` so bearer reads and refresh writes target
`sessions/<id>/token`.

## Rationale

After per-session token migration, the legacy token path may be a stub.
Session-bound calls need the session token first, and refresh must write the
new access token back to the same session rather than an ambient Claude instance
binding.

## Examples

### `jamsesh resume` durable session

**File**: `cmd/jamsesh/sessioncmd/resume.go:107`

```go
pc := &portalclient.Client{
	BaseURL:   portalURL,
	SessionID: sessionID,
}
portalclient.WireRefresh(pc)
```

### `jamsesh join --open` resume mint

**File**: `cmd/jamsesh/sessioncmd/join.go:172`

```go
mintPC := &portalclient.Client{
	BaseURL:   portalURL,
	SessionID: sessionID,
}
portalclient.WireRefresh(mintPC)
```

### Playground open path omits refresh

**File**: `cmd/jamsesh/sessioncmd/new.go:480`

```go
mintPC := &portalclient.Client{
	BaseURL:   baseURL,
	SessionID: resp.Session.Id,
}
// No refresh wiring for playground - anon bearers are non-refreshable.
```

## When to Use

- CLI code has resolved the target session before calling authenticated session endpoints.
- Refreshable durable session credentials are in play; call `portalclient.WireRefresh`.

## When NOT to Use

- Pre-binding account-level calls such as `/api/me`; leave `SessionID` empty.
- Anonymous playground bearers; set `SessionID` but do not wire refresh.

## Common Violations

- Calling `buildPortalClient()` for a playground resume/open path.
- Constructing `Client{BaseURL: ...}` for session-bound work after migration.
- Wiring refresh for non-refreshable anonymous bearers.

