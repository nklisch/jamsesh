// Package finalize: deterministic squash-message composition.
//
// The composer is pure: same input → same bytes, byte-for-byte, on every
// invocation. The bytes drive what the human sees in the plan preview and
// what `git commit -F -` consumes when finalize-run executes. Determinism
// is the contract.
package finalize

import (
	"strings"
	"unicode/utf8"

	"github.com/go-git/go-git/v5/plumbing/object"
)

// CoAuthor is one rendered Co-authored-by entry. AccountID is populated
// when the portal has a matching account for the commit author's email;
// empty otherwise. The Email casing is the first-seen casing across the
// selection (case-insensitive dedup uses lowercase as the map key).
type CoAuthor struct {
	Name      string
	Email     string
	AccountID string // empty when no portal account matches
}

// ComposeSquashMessage builds the squash-commit subject, body, and the
// distinct co-author list from a curated commit selection. The output is
// bytewise-deterministic for the same input.
//
// Subject rules:
//   - userOverrideSubject non-empty: use its first line truncated to 72 chars.
//   - else: use sessionGoal truncated at a word boundary to 72 chars, with
//     `…` appended when truncation happened.
//
// Body rules:
//   - empty when commits is empty.
//   - otherwise: a blank line followed by one `- <subject>` bullet per
//     commit in selection order, where `<subject>` is the first line of
//     each commit's message with whitespace trimmed.
//
// CoAuthors rules:
//   - dedup by strings.ToLower(email); preserve first-seen casing for the
//     rendered Name + Email; preserve first-appearance order.
//   - AccountID is left empty here — callers (plan.go) fill it in
//     best-effort via store.GetAccountByEmail.
func ComposeSquashMessage(sessionGoal, userOverrideSubject string, commits []*object.Commit) (subject, body string, coAuthors []CoAuthor) {
	subject = composeSubject(sessionGoal, userOverrideSubject)
	body = composeBody(commits)
	coAuthors = composeCoAuthors(commits)
	return subject, body, coAuthors
}

// composeSubject returns the squash subject line. See ComposeSquashMessage
// for the rules.
func composeSubject(sessionGoal, userOverride string) string {
	if userOverride != "" {
		first := firstLine(userOverride)
		return hardTruncate(first, 72)
	}
	return wordBoundaryTruncate(strings.TrimSpace(sessionGoal), 72)
}

// composeBody returns the "blank line + bulleted subjects" portion of the
// squash message. Returns "" when there are no commits.
func composeBody(commits []*object.Commit) string {
	if len(commits) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n")
	for _, c := range commits {
		b.WriteString("- ")
		b.WriteString(stripTrailersFromSubject(firstLine(c.Message)))
		b.WriteString("\n")
	}
	return b.String()
}

// composeCoAuthors collects the distinct authors from the selection in
// first-appearance order, dedup'd case-insensitively on email. The
// returned slice never contains entries with an empty email.
func composeCoAuthors(commits []*object.Commit) []CoAuthor {
	seen := make(map[string]struct{}, len(commits))
	out := make([]CoAuthor, 0, len(commits))
	for _, c := range commits {
		email := c.Author.Email
		if email == "" {
			continue
		}
		key := strings.ToLower(email)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, CoAuthor{
			Name:  c.Author.Name,
			Email: email,
		})
	}
	return out
}

// firstLine returns the substring up to the first newline, with surrounding
// whitespace trimmed. If there is no newline the whole string is trimmed
// and returned.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

// stripTrailersFromSubject removes a trailing `<Key>: <value>` segment from
// a subject line. Trailers are unusual on the first line — most commit
// subjects look like `feat: foo`, which is NOT a trailer (it's a
// conventional-commits prefix). This function is intentionally
// conservative: it returns the input as-is. The "strip trailers" behaviour
// requested by the story spec is a no-op on the first line of normal
// commit messages, and ambiguity around `feat:` vs `Signed-off-by:` makes
// any heuristic risky. The trailer-stripping that matters happens in
// firstLine (which discards everything after the first newline).
func stripTrailersFromSubject(s string) string {
	return s
}

// wordBoundaryTruncate truncates s to at most max runes. If s is already
// ≤ max runes long, it is returned unchanged (no ellipsis). Otherwise the
// function finds the last whitespace at or before position max and cuts
// there, appending `…`. If no whitespace exists within the limit (one
// giant word), it hard-cuts at max-1 runes and appends `…`.
func wordBoundaryTruncate(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}

	// Find the rune-index boundaries.
	runeIdx := 0
	lastSpaceByte := -1
	for i, r := range s {
		if runeIdx >= max {
			break
		}
		if r == ' ' || r == '\t' {
			lastSpaceByte = i
		}
		runeIdx++
	}

	if lastSpaceByte > 0 {
		return strings.TrimRight(s[:lastSpaceByte], " \t") + "…"
	}

	// One unbroken giant word: hard-cut at max-1 runes.
	cutBytes := 0
	count := 0
	for i := range s {
		if count >= max-1 {
			cutBytes = i
			break
		}
		count++
	}
	if cutBytes == 0 {
		cutBytes = len(s)
	}
	return s[:cutBytes] + "…"
}

// hardTruncate truncates s to at most max runes. If s is already short
// enough it is returned unchanged. Otherwise the function appends `…`
// after max-1 runes. Used for the user-override subject path where word-
// boundary handling is intentionally skipped (the user explicitly typed
// the subject).
func hardTruncate(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	count := 0
	for i := range s {
		if count >= max-1 {
			return s[:i] + "…"
		}
		count++
	}
	return s
}

// RenderSquashMessageBody composes the human-readable squash commit body
// from the pieces ComposeSquashMessage returns. Format:
//
//	<subject>
//	<body>            (already starts with a blank line)
//	                  (blank line)
//	Co-authored-by: ...
//	Co-authored-by: ...
//
// When coAuthors is empty, the trailing Co-authored-by block is omitted.
// When body is empty (no commits), the message is just <subject>.
//
// The output ends with a single trailing newline so that `git commit -F -`
// receives a well-formed message.
func RenderSquashMessageBody(subject, body string, coAuthors []CoAuthor) string {
	var b strings.Builder
	b.WriteString(subject)
	b.WriteString("\n")
	if body != "" {
		b.WriteString(body)
	}
	if len(coAuthors) > 0 {
		b.WriteString("\n")
		for _, ca := range coAuthors {
			b.WriteString("Co-authored-by: ")
			b.WriteString(ca.Name)
			b.WriteString(" <")
			b.WriteString(ca.Email)
			b.WriteString(">\n")
		}
	}
	return b.String()
}
