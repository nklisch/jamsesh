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

// TestWordlistLengthBand pins a lower bound on each embedded list so an
// accidental file truncation (a `git checkout` clobber, a bad sed, a CRLF
// re-encode that strips entries) is caught at CI time before it ships.
//
// 150 is roughly 85% of the smaller list (animals: ~182 at time of
// writing). Provides a meaningful guard without being a brittle exact-count
// assertion. (gate-tests-wordlist-diversity-threshold-and-length-band)
func TestWordlistLengthBand(t *testing.T) {
	const minEntries = 150
	if n := wordlist.AdjCount(); n < minEntries {
		t.Errorf("adjectives wordlist has %d entries; want >= %d (accidental truncation?)",
			n, minEntries)
	}
	if n := wordlist.AnimalCount(); n < minEntries {
		t.Errorf("animals wordlist has %d entries; want >= %d (accidental truncation?)",
			n, minEntries)
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
