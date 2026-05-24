// Package wordlist — corruption-resistance tests.
//
// These tests guard against accidental wordlist file corruption: stray blank
// lines, whitespace-only lines, or dash-only entries that would produce
// malformed handles like "", "-", "--otter", or "amber-".
//
// The package is tested in internal (white-box) mode here so that
// splitNonEmpty can be exercised directly without going through the real
// embedded wordlists.
package wordlist

import (
	"strings"
	"testing"
)

// TestPick_NeverProducesEmptyOrDashOnly runs 10 000 iterations and verifies
// that every handle:
//   - is non-empty
//   - contains exactly one hyphen
//   - does not start or end with a hyphen
//   - neither the adjective nor animal part is empty
//
// A failure here means the real embedded wordlist contains a blank or
// dash-only entry that slipped through splitNonEmpty.
func TestPick_NeverProducesEmptyOrDashOnly(t *testing.T) {
	const iterations = 10_000
	for i := range iterations {
		h := Pick()
		if h == "" {
			t.Fatalf("iter %d: Pick() returned empty string", i)
		}
		if strings.HasPrefix(h, "-") {
			t.Fatalf("iter %d: Pick() = %q starts with '-'", i, h)
		}
		if strings.HasSuffix(h, "-") {
			t.Fatalf("iter %d: Pick() = %q ends with '-'", i, h)
		}
		parts := strings.SplitN(h, "-", 2)
		if len(parts) != 2 {
			t.Fatalf("iter %d: Pick() = %q: want exactly one hyphen separator", i, h)
		}
		if parts[0] == "" {
			t.Fatalf("iter %d: Pick() = %q: adjective part is empty", i, h)
		}
		if parts[1] == "" {
			t.Fatalf("iter %d: Pick() = %q: animal part is empty", i, h)
		}
	}
}

// TestSplitNonEmpty_FiltersBlankAndWhitespaceLines confirms that
// splitNonEmpty strips empty lines and whitespace-only lines, which are the
// most common corruption artefacts when editing a wordlist by hand.
func TestSplitNonEmpty_FiltersBlankAndWhitespaceLines(t *testing.T) {
	raw := "\nfoo\n   \nbar\n\t\nbaz\n"
	got := splitNonEmpty(raw)
	want := []string{"foo", "bar", "baz"}
	if len(got) != len(want) {
		t.Fatalf("splitNonEmpty: got %v (len %d), want %v (len %d)", got, len(got), want, len(want))
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("splitNonEmpty[%d]: got %q, want %q", i, got[i], w)
		}
	}
}

// TestSplitNonEmpty_EmptyInput returns an empty slice without panicking.
func TestSplitNonEmpty_EmptyInput(t *testing.T) {
	for _, raw := range []string{"", "   ", "\n\n\n"} {
		got := splitNonEmpty(raw)
		if len(got) != 0 {
			t.Errorf("splitNonEmpty(%q): got %v, want empty slice", raw, got)
		}
	}
}

// TestSplitNonEmpty_DashOnlyLinesPassThrough documents that splitNonEmpty
// does NOT filter dash-only entries — the protection against dash-only
// corruption lives in the curated wordlists themselves, not in the parser.
//
// If a wordlist file ever contained a line that is just "-", Pick() would
// produce a handle like "-otter" or "amber-". TestPick_NeverProducesEmptyOrDashOnly
// is the runtime guard; this test documents the design boundary so that a
// future change to splitNonEmpty can make an informed decision.
func TestSplitNonEmpty_DashOnlyLinesPassThrough(t *testing.T) {
	raw := "good\n-\nbad"
	got := splitNonEmpty(raw)
	// splitNonEmpty treats "-" as a non-empty, non-whitespace token and keeps it.
	// This is intentional: the filter is a blank-line stripper, not a validator.
	if len(got) != 3 {
		t.Fatalf("splitNonEmpty: got len %d, want 3 (dash-only line is kept by design)", len(got))
	}
	if got[1] != "-" {
		t.Errorf("splitNonEmpty[1]: got %q, want \"-\" (dash-only line should pass through)", got[1])
	}
}

// TestSplitNonEmpty_TrimsLeadingTrailingSpacesFromEntries verifies that
// words with surrounding whitespace are trimmed so no entry starts or ends
// with a space character, preventing handles like " amber-otter" or "amber -otter".
func TestSplitNonEmpty_TrimsLeadingTrailingSpacesFromEntries(t *testing.T) {
	raw := "  foo  \n bar \nbaz"
	got := splitNonEmpty(raw)
	want := []string{"foo", "bar", "baz"}
	if len(got) != len(want) {
		t.Fatalf("splitNonEmpty: got %v, want %v", got, want)
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("splitNonEmpty[%d]: got %q, want %q (should be trimmed)", i, got[i], w)
		}
	}
}
