// Package githttp internal tests cover helpers that are not exported.
package githttp

// receive_pack_internal_test.go tests package-internal helpers from
// receive_pack.go that cannot be tested from the _test package.

import (
	"strings"
	"testing"
)

// TestLooksLikeReportStatus verifies the pkt-line report-status detector.
//
// The detector requires:
//  1. A valid 4-hex pkt-line length prefix whose numeric value matches the
//     actual first-packet size (pkt-line framing validated, not just hex).
//  2. The payload must contain "unpack " (the mandatory first line of any
//     git report-status). "ng " or "ok " alone are insufficient.
//
// Malformed/truncated input (bad framing, missing "unpack ") → false → 500.
func TestLooksLikeReportStatus(t *testing.T) {
	tests := []struct {
		name string
		buf  []byte
		want bool
	}{
		{
			name: "empty",
			buf:  []byte{},
			want: false,
		},
		{
			name: "too short (under 8 bytes)",
			buf:  []byte("003"),
			want: false,
		},
		{
			name: "nil buf",
			buf:  nil,
			want: false,
		},
		// Well-formed pkt-line report-status payloads (length matches actual data).
		{
			name: "valid: unpack ok only",
			buf:  buildFakeReportStatus("unpack ok\n"),
			want: true,
		},
		{
			name: "valid: unpack ok + ok ref",
			buf:  buildFakeReportStatus("unpack ok\nok refs/heads/main\n0000"),
			want: true,
		},
		{
			name: "valid: unpack ok + ng ref (hook rejection)",
			buf:  buildFakeReportStatus("unpack ok\nng refs/heads/main pre-receive hook declined\n0000"),
			want: true,
		},
		{
			name: "valid: real git rejection payload shape (contains unpack)",
			buf:  buildFakeReportStatus("unpack ok\nng refs/heads/main hook rejected"),
			want: true,
		},
		// Malformed: correct hex prefix but pkt-line length does NOT match actual buf size.
		{
			name: "malformed: length prefix does not match buf (truncated)",
			buf:  []byte("0030unpack ok\n0000"), // 0x30=48 but buf is only 18 bytes
			want: false,
		},
		{
			name: "malformed: uppercase hex but length mismatch",
			buf:  []byte("00ABunpack ok\n0000"), // 0xAB=171 but buf is only 18 bytes
			want: false,
		},
		// Non-hex prefix (subprocess crash output).
		{
			name: "non-hex prefix (crash/kill output)",
			buf:  []byte("fatal: bad object abc123\n"),
			want: false,
		},
		// Flush-pkt (0000) is not a report-status.
		{
			name: "flush pkt only (0000)",
			buf:  []byte("0000"),
			want: false,
		},
		// "ng " or "ok " alone without "unpack " — insufficient (could be garbage).
		{
			name: "ng only without unpack (malformed/truncated report)",
			buf:  buildFakeReportStatus("ng refs/heads/main pre-receive hook declined"),
			want: false,
		},
		{
			name: "ok only without unpack (malformed/truncated report)",
			buf:  buildFakeReportStatus("ok refs/heads/main"),
			want: false,
		},
		// Final-gate-2 regression (Finding 4): "unpack " appears in a LATER pkt-line,
		// not the first. The prior fix used strings.Contains over the whole buffer,
		// which passed this — the correct fix checks only the first pkt-line payload.
		{
			// First pkt-line body is "garbage\n"; "unpack " appears only in a
			// second pkt-line appended after it. Must return false: git report-status
			// requires the FIRST pkt-line to start with "unpack ".
			name: "unpack only in second pkt-line (first is garbage) — must be false",
			buf: func() []byte {
				// Build first pkt-line with garbage body.
				firstBody := "garbage junk line\n"
				first := buildFakeReportStatus(firstBody) // valid length prefix + body
				// Append a second pkt-line that starts with "unpack ok".
				secondBody := "unpack ok\n"
				secondLen := len(secondBody) + 4
				second := append([]byte(strings.ToUpper(formatHex(secondLen))), []byte(secondBody)...)
				return append(first, second...)
			}(),
			want: false,
		},
		{
			// A valid "unpack ok\n" first pkt-line followed by an ng line — regression
			// guard: this must still return true after the HasPrefix fix.
			name: "valid first pkt-line unpack ok followed by ng ref — must be true",
			buf:  buildFakeReportStatus("unpack ok\nng refs/heads/main hook rejected\n0000"),
			want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := looksLikeReportStatus(tc.buf)
			if got != tc.want {
				t.Errorf("looksLikeReportStatus(%q) = %v; want %v", string(tc.buf), got, tc.want)
			}
		})
	}
}

// buildFakeReportStatus builds a minimal pkt-line report-status payload
// for testing. It prepends a valid 4-hex length field.
func buildFakeReportStatus(body string) []byte {
	// pkt-line length includes the 4-byte length prefix itself.
	length := len(body) + 4
	prefix := []byte(strings.ToUpper(formatHex(length)))
	return append(prefix, []byte(body)...)
}

func formatHex(n int) string {
	return strings.ToUpper(
		// Manual 4-hex formatting
		string([]byte{hexDigit(n >> 12), hexDigit((n >> 8) & 0xf), hexDigit((n >> 4) & 0xf), hexDigit(n & 0xf)}))
}

func hexDigit(n int) byte {
	if n < 10 {
		return byte('0' + n)
	}
	return byte('a' + n - 10)
}
