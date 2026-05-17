package tokens

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

const rawTokenBytes = 32

// generateToken produces a cryptographically secure opaque token.
// It returns both the raw 64-char hex string (given to the caller) and its
// SHA-256 hex hash (stored at rest). The raw string is never persisted.
func generateToken() (raw, hash string, err error) {
	b := make([]byte, rawTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("tokens: read random: %w", err)
	}
	raw = hex.EncodeToString(b)
	sum := sha256.Sum256([]byte(raw))
	hash = hex.EncodeToString(sum[:])
	return raw, hash, nil
}

// hashToken deterministically hashes a raw token string using SHA-256.
// Used at validation time to look up the stored row by hash.
func hashToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
