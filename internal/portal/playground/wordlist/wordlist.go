// Package wordlist provides the two-word handle generator for playground
// sessions. Handles look like "amber-otter" — a calm/positive adjective
// followed by a recognisable animal, joined by a hyphen.
//
// The wordlists are embedded at compile time. Refreshing them requires a
// release, which is appropriate — curated wordlists don't change at runtime.
package wordlist

import (
	_ "embed"
	"math/rand/v2"
	"strings"
)

//go:embed adjectives.txt
var adjectivesRaw string

//go:embed animals.txt
var animalsRaw string

var (
	adjectives = splitNonEmpty(adjectivesRaw)
	animals    = splitNonEmpty(animalsRaw)
)

// Pick returns a fresh pronounceable handle like "amber-otter".
//
// Random selection uses math/rand/v2 — crypto-strength is not required here
// because handles are display values, not credentials. Per-session uniqueness
// is enforced at the join-transaction level with a collision-retry loop.
func Pick() string {
	a := adjectives[rand.IntN(len(adjectives))]
	n := animals[rand.IntN(len(animals))]
	return a + "-" + n
}

func splitNonEmpty(raw string) []string {
	out := make([]string, 0, 256)
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}
