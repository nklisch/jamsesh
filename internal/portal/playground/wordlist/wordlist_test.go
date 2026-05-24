package wordlist_test

import (
	"strings"
	"testing"

	"jamsesh/internal/portal/playground/wordlist"
)

func TestPick_Format(t *testing.T) {
	for range 50 {
		h := wordlist.Pick()
		parts := strings.SplitN(h, "-", 2)
		if len(parts) != 2 {
			t.Errorf("Pick() = %q: want exactly one hyphen", h)
			continue
		}
		if parts[0] == "" || parts[1] == "" {
			t.Errorf("Pick() = %q: both parts must be non-empty", h)
		}
	}
}

func TestPick_Diversity(t *testing.T) {
	// With ~239 adj × ~182 animals = ~43k+ combinations, 1000 picks should
	// yield many distinct handles. Requiring ≥ 900 distinct out of 1000 is a
	// conservative guard against a broken single-value wordlist.
	seen := make(map[string]bool, 1000)
	for range 1000 {
		seen[wordlist.Pick()] = true
	}
	if len(seen) < 900 {
		t.Errorf("only %d distinct handles from 1000 picks; wordlist may be too small", len(seen))
	}
}

func TestWordlistsNonEmpty(t *testing.T) {
	// If the embed fails or the file is empty, Pick() would panic. This test
	// exercises Pick() on startup and catches the panic as a failure.
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Pick() panicked: %v", r)
		}
	}()
	_ = wordlist.Pick()
}
