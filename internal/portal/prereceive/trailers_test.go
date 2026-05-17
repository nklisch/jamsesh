package prereceive

import (
	"testing"
)

func TestTrailers(t *testing.T) {
	t.Run("no trailer block", func(t *testing.T) {
		msg := "just a subject line"
		got := Trailers(msg)
		if got != nil {
			t.Errorf("expected nil, got %v", got)
		}
	})

	t.Run("single paragraph — no separator blank line", func(t *testing.T) {
		msg := "subject\n\nJam-Session: abc"
		// The last paragraph starts immediately after the blank line; but
		// there's nothing *before* the blank line that acts as a body, so
		// the block actually starts at index 0 after split... wait: "subject"
		// is paragraph 1, blank, then "Jam-Session: abc" is paragraph 2.
		// blockStart > 0, so this should parse.
		got := Trailers(msg)
		if got == nil {
			t.Fatal("expected trailers, got nil")
		}
		if got["Jam-Session"] != "abc" {
			t.Errorf("Jam-Session: want %q got %q", "abc", got["Jam-Session"])
		}
	})

	t.Run("all three required trailers present", func(t *testing.T) {
		msg := "Fix the thing\n\nSome body text here.\n\nJam-Session: sess-001\nJam-Turn: 3\nJam-Author: alice"
		got := Trailers(msg)
		if got == nil {
			t.Fatal("expected trailers, got nil")
		}
		cases := map[string]string{
			"Jam-Session": "sess-001",
			"Jam-Turn":    "3",
			"Jam-Author":  "alice",
		}
		for k, want := range cases {
			if got[k] != want {
				t.Errorf("%s: want %q got %q", k, want, got[k])
			}
		}
	})

	t.Run("trailers mixed with non-trailer lines rejected", func(t *testing.T) {
		msg := "subject\n\nJam-Session: s1\nnot a trailer\nJam-Author: bob"
		got := Trailers(msg)
		if got != nil {
			t.Errorf("expected nil (mixed block), got %v", got)
		}
	})

	t.Run("folded continuation", func(t *testing.T) {
		msg := "subject\n\nJam-Session: sess-abc\nJam-Turn: 1\nJam-Author: bob\n  extra-info"
		got := Trailers(msg)
		if got == nil {
			t.Fatal("expected trailers, got nil")
		}
		// Folded lines append to previous trailer value.
		if got["Jam-Author"] == "" {
			t.Error("Jam-Author should not be empty")
		}
	})

	t.Run("empty message", func(t *testing.T) {
		got := Trailers("")
		if got != nil {
			t.Errorf("expected nil for empty message, got %v", got)
		}
	})

	t.Run("first-occurrence wins for duplicate keys", func(t *testing.T) {
		msg := "subject\n\nJam-Session: first\nJam-Session: second\nJam-Turn: 1\nJam-Author: x"
		got := Trailers(msg)
		if got == nil {
			t.Fatal("expected trailers")
		}
		if got["Jam-Session"] != "first" {
			t.Errorf("want first, got %q", got["Jam-Session"])
		}
	})
}

func TestCheckRequiredTrailers(t *testing.T) {
	required := []string{"Jam-Session", "Jam-Turn", "Jam-Author"}

	t.Run("all present", func(t *testing.T) {
		msg := "subject\n\nJam-Session: s1\nJam-Turn: 1\nJam-Author: alice"
		missing := CheckRequiredTrailers(msg, required)
		if len(missing) != 0 {
			t.Errorf("expected no missing, got %v", missing)
		}
	})

	t.Run("Jam-Session absent", func(t *testing.T) {
		msg := "subject\n\nJam-Turn: 1\nJam-Author: alice"
		missing := CheckRequiredTrailers(msg, required)
		if len(missing) != 1 || missing[0] != "Jam-Session" {
			t.Errorf("expected [Jam-Session] missing, got %v", missing)
		}
	})

	t.Run("Jam-Turn absent", func(t *testing.T) {
		msg := "subject\n\nJam-Session: s1\nJam-Author: alice"
		missing := CheckRequiredTrailers(msg, required)
		if len(missing) != 1 || missing[0] != "Jam-Turn" {
			t.Errorf("expected [Jam-Turn] missing, got %v", missing)
		}
	})

	t.Run("Jam-Author absent", func(t *testing.T) {
		msg := "subject\n\nJam-Session: s1\nJam-Turn: 2"
		missing := CheckRequiredTrailers(msg, required)
		if len(missing) != 1 || missing[0] != "Jam-Author" {
			t.Errorf("expected [Jam-Author] missing, got %v", missing)
		}
	})

	t.Run("all absent — no trailer block", func(t *testing.T) {
		msg := "subject line only"
		missing := CheckRequiredTrailers(msg, required)
		if len(missing) != 3 {
			t.Errorf("expected 3 missing, got %v", missing)
		}
	})

	t.Run("trailer block with non-trailer line disqualifies", func(t *testing.T) {
		msg := "subject\n\nJam-Session: s1\nbad line\nJam-Author: bob"
		missing := CheckRequiredTrailers(msg, required)
		// Block is rejected, so all three are missing.
		if len(missing) != 3 {
			t.Errorf("expected 3 missing (bad block), got %v", missing)
		}
	})

	t.Run("trailer present but empty value treated as missing", func(t *testing.T) {
		// The regex requires at least one char after ": ", so an empty value
		// won't parse as a trailer line — it counts as missing.
		msg := "subject\n\nJam-Session: s1\nJam-Turn: \nJam-Author: bob"
		// "Jam-Turn: " (with trailing space) — value after trim is empty... but
		// actually "\s+(.+)" requires at least one non-space char in value.
		// "Jam-Turn: " won't match — the whole block is disqualified.
		missing := CheckRequiredTrailers(msg, required)
		// All 3 are missing because the block is poisoned.
		if len(missing) == 0 {
			t.Error("expected some missing trailers")
		}
	})
}
