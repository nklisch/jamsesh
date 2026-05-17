package prereceive

import (
	"strings"
	"testing"
)

// FuzzCommitTrailerParse exercises the Trailers and CheckRequiredTrailers
// functions with arbitrary commit message bytes.
//
// Properties asserted:
//  1. Neither function panics on any input (the fuzz runner catches panics
//     automatically and saves the seed).
//  2. If Trailers returns a non-nil map, every key must be non-empty and every
//     value must be non-empty (the regex already guarantees this, but we
//     assert it explicitly as a regression guard).
//  3. CheckRequiredTrailers never returns fewer missing keys than the keys
//     actually absent from the Trailers result — i.e. it never silently drops
//     a required key.
//  4. When Trailers returns nil (no trailer block found), CheckRequiredTrailers
//     must report all required keys as missing.
func FuzzCommitTrailerParse(f *testing.F) {
	// Seed: well-formed messages with all jamsesh trailers.
	f.Add([]byte("Fix the thing\n\nJam-Session: ses_abc\nJam-Turn: turn_001\nJam-Author: alice\n"))
	f.Add([]byte("subject\n\nJam-Session: sess-001\nJam-Turn: 3\nJam-Author: alice\nResolves-Conflict: true\nAuto-Merger: false\nSource-Commit: deadbeef\n"))
	f.Add([]byte("Multi paragraph\n\nSome body.\nMore body.\n\nJam-Session: sess-xyz\nJam-Turn: 42\nJam-Author: bob\n"))

	// Seed: folded trailer continuation line.
	f.Add([]byte("subject\n\nJam-Session: sess-abc\nJam-Turn: 1\nJam-Author: bob\n  extra-info\n"))

	// Seed: duplicate key — first occurrence wins.
	f.Add([]byte("subject\n\nJam-Session: first\nJam-Session: second\nJam-Turn: 1\nJam-Author: x\n"))

	// Seed: known-bad — missing all trailers.
	f.Add([]byte("subject\n\nMalformed-Trailer"))
	f.Add([]byte("no trailers here"))
	f.Add([]byte(""))

	// Seed: known-bad — non-trailer in last paragraph disqualifies block.
	f.Add([]byte("subject\n\nJam-Session: s1\nnot a trailer\nJam-Author: bob\n"))

	// Seed: only a subject, no blank line before trailer block.
	f.Add([]byte("Jam-Session: abc\nJam-Turn: 1\nJam-Author: x"))

	// Seed: trailer block with blank line in the middle (disqualified).
	f.Add([]byte("subject\n\nJam-Session: abc\n\nJam-Turn: 1\nJam-Author: x\n"))

	// Seed: Windows CRLF line endings.
	f.Add([]byte("subject\r\n\r\nJam-Session: sess\r\nJam-Turn: 1\r\nJam-Author: alice\r\n"))

	// Seed: extremely long value.
	f.Add([]byte("subject\n\nJam-Session: " + strings.Repeat("x", 1024) + "\nJam-Turn: 1\nJam-Author: alice\n"))

	required := []string{"Jam-Session", "Jam-Turn", "Jam-Author", "Resolves-Conflict", "Auto-Merger", "Source-Commit"}

	f.Fuzz(func(t *testing.T, input []byte) {
		msg := string(input)

		// Property 1 (no panic) is enforced by the fuzz runner itself.

		trailers := Trailers(msg)

		// Property 2: all returned keys and values must be non-empty strings.
		if trailers != nil {
			for k, v := range trailers {
				if k == "" {
					t.Errorf("Trailers returned empty key for input %q", msg)
				}
				if v == "" {
					t.Errorf("Trailers returned empty value for key %q, input %q", k, msg)
				}
			}
		}

		missing := CheckRequiredTrailers(msg, required)

		// Property 3: CheckRequiredTrailers must not silently drop required keys.
		// For each key in required, if it is absent from the trailers map (or
		// the map is nil), that key must appear in missing.
		for _, key := range required {
			val, ok := trailers[key]
			absentOrEmpty := !ok || strings.TrimSpace(val) == ""
			if absentOrEmpty {
				found := false
				for _, m := range missing {
					if m == key {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("CheckRequiredTrailers silently dropped absent key %q; input=%q", key, msg)
				}
			}
		}

		// Property 4: when Trailers returns nil, all required keys must be missing.
		if trailers == nil {
			if len(missing) != len(required) {
				t.Errorf("Trailers=nil but CheckRequiredTrailers reported %d missing (want %d); input=%q",
					len(missing), len(required), msg)
			}
		}
	})
}
