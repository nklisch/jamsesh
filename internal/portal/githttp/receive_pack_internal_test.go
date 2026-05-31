// Package githttp internal tests cover helpers that are not exported.
package githttp

// receive_pack_internal_test.go tests package-internal helpers from
// receive_pack.go that cannot be tested from the _test package.

import (
	"strings"
	"testing"
)

// TestLooksLikeReportStatus verifies the pkt-line report-status detector.
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
			name: "too short",
			buf:  []byte("003"),
			want: false,
		},
		{
			name: "valid pktline with unpack ok",
			buf:  []byte("0030unpack ok\n0000"),
			want: true,
		},
		{
			name: "valid pktline with ng line",
			buf:  []byte("001e\x01unpack ok\n002eng refs/heads/main hook rejected\n0000"),
			want: true,
		},
		{
			name: "valid pktline with ok ref",
			buf:  []byte("001e\x01unpack ok\n001aok refs/heads/main\n0000"),
			want: true,
		},
		{
			name: "non-hex prefix (crash/kill output)",
			buf:  []byte("fatal: bad object abc123\n"),
			want: false,
		},
		{
			name: "hex prefix but no keywords",
			buf:  []byte("0000"),
			want: false,
		},
		{
			name: "contains unpack keyword",
			buf:  append([]byte("0030"), []byte("unpack ok\n0000")...),
			want: true,
		},
		{
			name: "real git rejection payload shape",
			buf:  buildFakeReportStatus("ng refs/heads/main pre-receive hook declined"),
			want: true,
		},
		{
			name: "nil buf",
			buf:  nil,
			want: false,
		},
		{
			name: "uppercase hex prefix + unpack",
			buf:  []byte("00AB" + "unpack ok\n0000"),
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
