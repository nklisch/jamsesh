package tokens

import (
	"testing"
)

func TestGenerateToken_Length(t *testing.T) {
	raw, hash, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken: %v", err)
	}
	// 32 bytes → 64 hex chars
	if len(raw) != 64 {
		t.Errorf("raw token length: want 64, got %d", len(raw))
	}
	// SHA-256 → 32 bytes → 64 hex chars
	if len(hash) != 64 {
		t.Errorf("hash length: want 64, got %d", len(hash))
	}
}

func TestGenerateToken_Entropy(t *testing.T) {
	raw1, _, err := generateToken()
	if err != nil {
		t.Fatalf("first generateToken: %v", err)
	}
	raw2, _, err := generateToken()
	if err != nil {
		t.Fatalf("second generateToken: %v", err)
	}
	if raw1 == raw2 {
		t.Error("consecutive generateToken calls produced the same token (entropy failure)")
	}
}

func TestHashToken_Deterministic(t *testing.T) {
	const input = "deadbeef1234567890abcdef"
	h1 := hashToken(input)
	h2 := hashToken(input)
	if h1 != h2 {
		t.Errorf("hashToken not deterministic: %q != %q", h1, h2)
	}
	if len(h1) != 64 {
		t.Errorf("hash length: want 64, got %d", len(h1))
	}
}

func TestHashToken_DifferentInputs(t *testing.T) {
	h1 := hashToken("aaaa")
	h2 := hashToken("bbbb")
	if h1 == h2 {
		t.Error("different inputs produced same hash")
	}
}

func TestGenerateToken_HashMatchesHashToken(t *testing.T) {
	raw, hash, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken: %v", err)
	}
	if got := hashToken(raw); got != hash {
		t.Errorf("generateToken hash mismatch: got %q, want %q", got, hash)
	}
}
