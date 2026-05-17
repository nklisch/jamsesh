package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
)

func TestGeneratePKCE(t *testing.T) {
	pair, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE() error: %v", err)
	}

	// Verifier must be non-empty and contain only base64url characters.
	if pair.Verifier == "" {
		t.Error("Verifier is empty")
	}
	if strings.ContainsAny(pair.Verifier, "+/=") {
		t.Errorf("Verifier contains non-base64url characters: %q", pair.Verifier)
	}

	// Verifier decodes to exactly 32 bytes.
	raw, err := base64.RawURLEncoding.DecodeString(pair.Verifier)
	if err != nil {
		t.Fatalf("Verifier is not valid base64url: %v", err)
	}
	if len(raw) != 32 {
		t.Errorf("Verifier encodes %d bytes, want 32", len(raw))
	}

	// Challenge must equal base64url(sha256(Verifier)) without padding.
	sum := sha256.Sum256([]byte(pair.Verifier))
	wantChallenge := base64.RawURLEncoding.EncodeToString(sum[:])
	if pair.Challenge != wantChallenge {
		t.Errorf("Challenge = %q, want %q", pair.Challenge, wantChallenge)
	}

	// Challenge must not contain base64 padding.
	if strings.Contains(pair.Challenge, "=") {
		t.Errorf("Challenge contains padding: %q", pair.Challenge)
	}
}

func TestGeneratePKCEUniqueness(t *testing.T) {
	a, err := GeneratePKCE()
	if err != nil {
		t.Fatal(err)
	}
	b, err := GeneratePKCE()
	if err != nil {
		t.Fatal(err)
	}
	if a.Verifier == b.Verifier {
		t.Error("consecutive GeneratePKCE calls returned the same verifier")
	}
}

func TestGenerateState(t *testing.T) {
	s, err := GenerateState()
	if err != nil {
		t.Fatalf("GenerateState() error: %v", err)
	}
	if s == "" {
		t.Error("state is empty")
	}
	if strings.ContainsAny(s, "+/=") {
		t.Errorf("state contains non-base64url characters: %q", s)
	}

	// Decodes to exactly 16 bytes.
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		t.Fatalf("state is not valid base64url: %v", err)
	}
	if len(raw) != 16 {
		t.Errorf("state encodes %d bytes, want 16", len(raw))
	}
}

func TestGenerateStateUniqueness(t *testing.T) {
	a, err := GenerateState()
	if err != nil {
		t.Fatal(err)
	}
	b, err := GenerateState()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Error("consecutive GenerateState calls returned the same nonce")
	}
}
