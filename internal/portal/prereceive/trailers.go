package prereceive

import (
	"regexp"
	"strings"
)

// trailerRe matches a single trailer line: key starts with a letter, then
// letters/digits/hyphens/underscores, followed by ": " and a non-empty value.
//
// Note: the key character set here (allowing underscore) is a small
// superset of git-interpret-trailers' strict set ([A-Za-z][A-Za-z0-9-]*).
// Underscore is permitted defensively; enforcement of the exact jamsesh key
// names is left to CheckRequiredTrailers.
var trailerRe = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9_-]*):\s+(.+)$`)

// Trailers parses the trailer block from a raw git commit message and returns
// a map of Key → Value. The map is nil if no trailer block is found.
//
// Parsing rule (simplified from git-interpret-trailers(1)):
//   - Split the message into paragraphs separated by blank lines.
//   - Take the LAST paragraph.
//   - If every line in that paragraph matches the Key: value pattern
//     (or is a folded continuation starting with whitespace), treat it
//     as the trailer block.
//   - Any mid-block blank line or non-trailer line disqualifies the block.
//
// This is the strict form: the last paragraph must be entirely well-formed
// trailer lines. jamsesh's CC-plugin teaching skill instructs agents to emit
// trailers in this exact form, so the simplification is acceptable for v1.
func Trailers(message string) map[string]string {
	trailers := parseTrailers(message)
	if trailers == nil {
		return nil
	}
	m := make(map[string]string, len(trailers))
	for _, t := range trailers {
		// First occurrence wins (consistent with git-interpret-trailers).
		if _, exists := m[t.key]; !exists {
			m[t.key] = t.value
		}
	}
	return m
}

// trailer is an internal key/value pair used during parsing.
type trailer struct {
	key   string
	value string
}

// parseTrailers returns the raw ordered trailer list from the last paragraph
// of message, or nil if no valid trailer block exists.
func parseTrailers(message string) []trailer {
	// Normalise line endings and strip trailing newlines.
	msg := strings.ReplaceAll(message, "\r\n", "\n")
	msg = strings.TrimRight(msg, "\n")
	if msg == "" {
		return nil
	}

	lines := strings.Split(msg, "\n")

	// Find the start of the last paragraph (block of non-blank lines at the
	// end of the message).
	blockStart := len(lines)
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) == "" {
			break
		}
		blockStart = i
	}

	// If the whole message is one paragraph (no blank lines), there's nothing
	// before the block — that means there's no blank-line separator, so no
	// trailer block.
	if blockStart == 0 {
		return nil
	}

	block := lines[blockStart:]
	if len(block) == 0 {
		return nil
	}

	var out []trailer
	for _, l := range block {
		if l == "" {
			// A blank line inside the supposed trailer block disqualifies it.
			return nil
		}
		// Folded continuation: line starts with whitespace and we already have
		// at least one trailer entry to append to.
		if len(l) > 0 && (l[0] == ' ' || l[0] == '\t') && len(out) > 0 {
			out[len(out)-1].value += " " + strings.TrimSpace(l)
			continue
		}
		m := trailerRe.FindStringSubmatch(l)
		if m == nil {
			// Not a trailer line — block is disqualified.
			return nil
		}
		out = append(out, trailer{key: m[1], value: m[2]})
	}
	return out
}

// CheckRequiredTrailers checks that every key in required is present in the
// commit message's trailer block and has a non-empty value. It returns the
// names of any missing or empty-value trailers.
//
// A trailer is considered missing if either:
//   - It is not present in the trailer block at all, OR
//   - It is present but its value is empty (after trimming).
func CheckRequiredTrailers(message string, required []string) (missing []string) {
	have := Trailers(message) // nil if no trailer block

	for _, key := range required {
		val, ok := have[key]
		if !ok || strings.TrimSpace(val) == "" {
			missing = append(missing, key)
		}
	}
	return missing
}
