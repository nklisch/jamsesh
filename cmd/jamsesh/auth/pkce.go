// Package auth implements the "auth" subcommand for the jamsesh CLI.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

// PKCEPair holds a PKCE code_verifier and the derived code_challenge (S256).
type PKCEPair struct {
	// Verifier is the raw code_verifier: 32 random bytes, base64url-encoded
	// (no padding), per RFC 7636 §4.1.
	Verifier string
	// Challenge is base64url(sha256(Verifier)), sent to the authorization
	// endpoint as code_challenge with code_challenge_method=S256.
	Challenge string
}

// GeneratePKCE creates a fresh PKCE pair. Returns an error if the OS
// entropy source fails.
func GeneratePKCE() (PKCEPair, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return PKCEPair{}, fmt.Errorf("generating PKCE verifier: %w", err)
	}
	verifier := base64.RawURLEncoding.EncodeToString(raw)

	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	return PKCEPair{Verifier: verifier, Challenge: challenge}, nil
}

// GenerateState returns a cryptographically random state nonce for OAuth
// CSRF protection. The nonce is 16 random bytes, base64url-encoded (no
// padding).
func GenerateState() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generating state nonce: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}
