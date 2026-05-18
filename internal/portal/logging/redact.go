package logging

import (
	"net/url"
	"regexp"
	"strings"
)

// sensitiveParams is the set of query parameter names (lower-cased) whose
// values are redacted from log output. The set covers magic-link tokens,
// OAuth state/code, and ticket-based auth flows documented in the portal spec.
var sensitiveParams = map[string]bool{
	"token":  true,
	"code":   true,
	"state":  true,
	"ticket": true,
}

// sensitiveParamRE matches any of the sensitive parameter names (case-insensitive)
// in a raw query string for the fallback regex path.
var sensitiveParamRE = regexp.MustCompile(`(?i)((?:^|&)(token|code|state|ticket)=)[^&]*`)

// RedactQueryTokens replaces the values of sensitive query parameters
// (token, code, state, ticket) with "<redacted>" in any URL-shaped string.
// Returns the input unchanged if it has no '?' or parsing fails cleanly
// (the fallback regex path is used when url.ParseQuery returns an error to
// avoid ever returning a raw token value).
//
// The function accepts either:
//   - a full URL string (e.g. "/auth/callback?code=abc&state=xyz"), or
//   - a raw query string without the leading '?' (e.g. "code=abc&state=xyz").
//
// In both cases the return value preserves the input shape.
func RedactQueryTokens(s string) string {
	if s == "" {
		return s
	}

	// Determine whether s contains a '?' — if so it is a URL-shaped string
	// and we need to split on it and reconstruct.
	qIdx := strings.IndexByte(s, '?')
	var prefix, rawQuery string
	if qIdx >= 0 {
		prefix = s[:qIdx+1] // include the '?'
		rawQuery = s[qIdx+1:]
	} else {
		// Treat the whole string as a raw query.
		prefix = ""
		rawQuery = s
	}

	if rawQuery == "" {
		return s
	}

	redacted, ok := redactRawQuery(rawQuery)
	if !ok {
		// url.ParseQuery failed; fall back to regex to avoid returning raw values.
		redacted = sensitiveParamRE.ReplaceAllString(rawQuery, "${1}<redacted>")
	}
	return prefix + redacted
}

// redactRawQuery walks the key=value pairs of a raw query string, replaces the
// values of sensitive params with the literal string <redacted>, and
// reconstructs the query string. The reconstruction preserves pair order and
// keeps non-sensitive values percent-encoded as they were in the original.
// Returns the redacted query and true on success, or ("", false) if any
// parsing error is encountered.
func redactRawQuery(rawQuery string) (string, bool) {
	// url.ParseQuery to validate; we'll re-walk the raw string for output so
	// we don't re-encode already-encoded values.
	if _, err := url.ParseQuery(rawQuery); err != nil {
		return "", false
	}

	var b strings.Builder
	for rawQuery != "" {
		var pair string
		if i := strings.IndexByte(rawQuery, '&'); i >= 0 {
			pair, rawQuery = rawQuery[:i], rawQuery[i+1:]
		} else {
			pair, rawQuery = rawQuery, ""
		}
		if b.Len() > 0 {
			b.WriteByte('&')
		}
		eqIdx := strings.IndexByte(pair, '=')
		if eqIdx < 0 {
			// Key-only param; decode to check sensitivity but emit as-is.
			key, _ := url.QueryUnescape(pair)
			if sensitiveParams[strings.ToLower(key)] {
				b.WriteString(pair)
				b.WriteString("=<redacted>")
			} else {
				b.WriteString(pair)
			}
			continue
		}
		rawKey, rawVal := pair[:eqIdx], pair[eqIdx+1:]
		key, _ := url.QueryUnescape(rawKey)
		if sensitiveParams[strings.ToLower(key)] {
			b.WriteString(rawKey)
			b.WriteString("=<redacted>")
		} else {
			b.WriteString(rawKey)
			b.WriteByte('=')
			b.WriteString(rawVal)
		}
	}
	return b.String(), true
}
