# Opaque Token Hash-At-Rest

Opaque bearer, invite, magic-link, and resume credentials are generated as raw
random tokens for the caller but persisted and looked up only by SHA-256 hex
hash.

## Rationale

These tokens are bearer credentials. Persisting only `token_hash` keeps database
leaks from exposing immediately usable credentials while still allowing
deterministic lookup during exchange or validation.

## Examples

### Shared bearer token helper

**File**: `internal/portal/tokens/token.go:12`

```go
func generateToken() (raw, hash string, err error) {
	b := make([]byte, rawTokenBytes)
	_, err = rand.Read(b)
	raw = hex.EncodeToString(b)
	sum := sha256.Sum256([]byte(raw))
	hash = hex.EncodeToString(sum[:])
	return raw, hash, nil
}
```

### Magic-link token stored hashed, raw sent in fragment

**File**: `internal/portal/auth/magic_link.go:105`

```go
raw, hash, err := generateMagicToken()
// ...
TokenHash: hash,
// ...
magicURL := h.portalURL + "/auth/magic-link#token=" + raw
```

### Session-resume token stored hashed, raw embedded in resume URL

**File**: `internal/portal/sessionresume/mint.go:82`

```go
rawToken, tokenHash, err := generateResumeToken()
// ...
TokenHash: tokenHash,
// ...
resumeURL, err := composeResumeURL(h.portalURL, orgID, sessionID, rawToken)
```

## When to Use

- Any new bearer, invite, magic-link, resume, or other opaque one-time credential.
- Any server-side lookup where the client submits the raw token back for validation.

## When NOT to Use

- Public IDs that are not credentials.
- Human passwords; use a salted password KDF, not raw SHA-256.

## Common Violations

- Persisting the raw token in a table or log.
- Looking up by raw token instead of `hash(raw)`.
- Returning both a URL containing the raw token and a separate raw-token field.

