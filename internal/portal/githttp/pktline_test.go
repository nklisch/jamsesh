package githttp

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"strings"
	"testing"

	"jamsesh/internal/portal/prereceive"
)

// buildPktLine returns a pkt-line encoded string for the given payload.
// Exported for readability; payload ends with \n per git convention.
func buildPktLine(payload string) string {
	return fmt.Sprintf("%04x%s", len(payload)+4, payload)
}

// buildCommandList builds a synthetic receive-pack command list as a byte
// slice. Each update is formatted as a pkt-line command. A "0000" flush
// terminates the list; remaining bytes represent the pack data.
func buildTestCommandList(updates []prereceive.RefUpdate, packData []byte) []byte {
	var buf bytes.Buffer
	for i, u := range updates {
		oldSHA := u.OldSHA
		if oldSHA == "" {
			oldSHA = strings.Repeat("0", 40)
		}
		line := oldSHA + " " + u.NewSHA + " " + u.Ref + "\n"
		if i == 0 {
			// First line carries capability: strip is tested by stripping NUL.
			line = oldSHA + " " + u.NewSHA + " " + u.Ref + "\x00side-band-64k\n"
		}
		totalLen := len(line) + 4
		fmt.Fprintf(&buf, "%04x%s", totalLen, line)
	}
	buf.WriteString("0000")
	buf.Write(packData)
	return buf.Bytes()
}

// TestReadCommandList_SingleUpdate parses a single ref-update command list.
// buildTestCommandList inserts "side-band-64k" in the first line's capability
// string, so we also assert the returned cap set contains "side-band-64k".
func TestReadCommandList_SingleUpdate(t *testing.T) {
	oldSHA := strings.Repeat("a", 40)
	newSHA := strings.Repeat("b", 40)
	ref := "refs/heads/jam/sess-1/acc-1/main"
	packData := []byte("PACK\x00\x00\x00\x02")

	raw := buildTestCommandList([]prereceive.RefUpdate{
		{OldSHA: oldSHA, NewSHA: newSHA, Ref: ref},
	}, packData)

	updates, caps, packReader, err := readCommandList(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("readCommandList: %v", err)
	}

	if len(updates) != 1 {
		t.Fatalf("want 1 update, got %d", len(updates))
	}
	u := updates[0]
	if u.OldSHA != oldSHA {
		t.Errorf("OldSHA: want %q, got %q", oldSHA, u.OldSHA)
	}
	if u.NewSHA != newSHA {
		t.Errorf("NewSHA: want %q, got %q", newSHA, u.NewSHA)
	}
	if u.Ref != ref {
		t.Errorf("Ref: want %q, got %q", ref, u.Ref)
	}

	// The test helper writes "side-band-64k" in the first line capability string;
	// assert the parsed cap set contains it.
	if !caps["side-band-64k"] {
		t.Errorf("caps: want side-band-64k to be set, got caps=%v", caps)
	}

	// Remaining bytes should be the pack data.
	gotPack, err := io.ReadAll(packReader)
	if err != nil {
		t.Fatalf("read pack: %v", err)
	}
	if !bytes.Equal(gotPack, packData) {
		t.Errorf("pack data mismatch: want %q, got %q", packData, gotPack)
	}
}

// TestReadCommandList_MultipleUpdates parses multiple ref-update lines.
func TestReadCommandList_MultipleUpdates(t *testing.T) {
	shas := []string{
		strings.Repeat("a", 40),
		strings.Repeat("b", 40),
		strings.Repeat("c", 40),
	}
	updates := []prereceive.RefUpdate{
		{OldSHA: shas[0], NewSHA: shas[1], Ref: "refs/heads/jam/sess-1/acc-1/feat-a"},
		{OldSHA: shas[1], NewSHA: shas[2], Ref: "refs/heads/jam/sess-1/acc-1/feat-b"},
	}

	raw := buildTestCommandList(updates, nil)

	got, _, _, err := readCommandList(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("readCommandList: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("want 2 updates, got %d", len(got))
	}
	for i, u := range got {
		if u.Ref != updates[i].Ref {
			t.Errorf("updates[%d].Ref: want %q, got %q", i, updates[i].Ref, u.Ref)
		}
		if u.NewSHA != updates[i].NewSHA {
			t.Errorf("updates[%d].NewSHA: want %q, got %q", i, updates[i].NewSHA, u.NewSHA)
		}
	}
}

// TestReadCommandList_NewRef verifies that an all-zeros OldSHA is preserved
// (new ref creation).
func TestReadCommandList_NewRef(t *testing.T) {
	newSHA := strings.Repeat("b", 40)
	ref := "refs/heads/jam/sess-1/acc-1/new-branch"

	// OldSHA="" in prereceive.RefUpdate means new ref; we encode as 40 zeros.
	raw := buildTestCommandList([]prereceive.RefUpdate{
		{OldSHA: "", NewSHA: newSHA, Ref: ref},
	}, nil)

	updates, _, _, err := readCommandList(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("readCommandList: %v", err)
	}

	if len(updates) != 1 {
		t.Fatalf("want 1 update, got %d", len(updates))
	}
	// 40 zeros on the wire → empty OldSHA (new-ref sentinel).
	if updates[0].OldSHA != "" {
		t.Errorf("OldSHA for new ref: want empty string, got %q", updates[0].OldSHA)
	}
}

// TestReadCommandList_FlushOnly verifies that an immediate flush returns
// an empty update list.
func TestReadCommandList_FlushOnly(t *testing.T) {
	raw := []byte("0000")
	updates, _, _, err := readCommandList(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("readCommandList on flush-only: %v", err)
	}
	if len(updates) != 0 {
		t.Errorf("want 0 updates from flush-only, got %d", len(updates))
	}
}

// TestReadCommandList_InvalidHex verifies that a bad hex prefix returns an error.
func TestReadCommandList_InvalidHex(t *testing.T) {
	raw := []byte("xxxx")
	_, _, _, err := readCommandList(bytes.NewReader(raw))
	if err == nil {
		t.Error("expected error for invalid hex prefix, got nil")
	}
}

// TestWriteReportStatusRejection_NoSideband verifies that all updates are
// written as plain "ng" lines when no sideband capability is present
// (backward-compat: raw pkt-lines without outer wrap).
func TestWriteReportStatusRejection_NoSideband(t *testing.T) {
	updates := []prereceive.RefUpdate{
		{Ref: "refs/heads/jam/sess-1/acc-1/main", OldSHA: strings.Repeat("a", 40), NewSHA: strings.Repeat("b", 40)},
		{Ref: "refs/heads/jam/sess-1/acc-1/feat", OldSHA: strings.Repeat("b", 40), NewSHA: strings.Repeat("c", 40)},
	}
	rejections := []prereceive.Rejection{
		{Code: prereceive.CodeMissingTrailer, Message: "missing required trailers", Details: map[string]any{}},
	}

	var buf bytes.Buffer
	writeReportStatusRejection(&buf, updates, rejections, map[string]bool{})

	out := buf.String()

	// Must contain unpack ok pkt-line directly (no sideband wrapping).
	if !strings.Contains(out, "unpack ok") {
		t.Error("report-status should contain 'unpack ok'")
	}

	// Both refs must have "ng" lines directly in the output.
	for _, u := range updates {
		if !strings.Contains(out, "ng "+u.Ref) {
			t.Errorf("report-status missing 'ng %s' line", u.Ref)
		}
	}

	// Must end with flush packet.
	if !strings.HasSuffix(out, "0000") {
		t.Error("report-status should end with '0000' flush packet")
	}
}

// TestWriteReportStatusRejection_PerRefReason verifies that per-ref rejection
// details are used when the rejection's Details["ref"] matches an update.
func TestWriteReportStatusRejection_PerRefReason(t *testing.T) {
	ref := "refs/heads/jam/sess-1/acc-1/main"
	updates := []prereceive.RefUpdate{
		{Ref: ref, OldSHA: strings.Repeat("a", 40), NewSHA: strings.Repeat("b", 40)},
	}
	rejections := []prereceive.Rejection{
		{
			Code:    prereceive.CodeRefNamespaceViolation,
			Message: "ref namespace violation",
			Details: map[string]any{"ref": ref},
		},
	}

	var buf bytes.Buffer
	writeReportStatusRejection(&buf, updates, rejections, map[string]bool{})

	out := buf.String()
	if !strings.Contains(out, "ref namespace violation") {
		t.Errorf("expected per-ref reason in output, got:\n%s", out)
	}
}

// TestWriteReportStatusRejection_SidebandWrap verifies that when the client
// negotiates side-band-64k, report-status pkt-lines are wrapped in outer
// sideband packets on band 1 (\x01). This is the core fix for the
// "bad band #117" regression (git reads the 'u' in "unpack ok" as band 117).
func TestWriteReportStatusRejection_SidebandWrap(t *testing.T) {
	ref := "refs/heads/jam/sess-1/acc-1/main"
	updates := []prereceive.RefUpdate{
		{Ref: ref, OldSHA: strings.Repeat("a", 40), NewSHA: strings.Repeat("b", 40)},
	}
	rejections := []prereceive.Rejection{
		{Code: prereceive.CodeMissingTrailer, Message: "missing required trailers", Details: map[string]any{}},
	}
	caps := map[string]bool{"side-band-64k": true}

	var buf bytes.Buffer
	writeReportStatusRejection(&buf, updates, rejections, caps)

	raw := buf.Bytes()

	// Helper: parse one outer pkt-line from raw[offset:].
	// Returns (payload bytes, new offset, error).
	parseOuterPktLine := func(data []byte, offset int) (payload []byte, next int, err error) {
		if offset+4 > len(data) {
			return nil, offset, fmt.Errorf("not enough bytes for length prefix at offset %d", offset)
		}
		lenHex := string(data[offset : offset+4])
		if lenHex == "0000" {
			return nil, offset + 4, nil // flush packet
		}
		var n int
		if _, err := fmt.Sscanf(lenHex, "%04x", &n); err != nil {
			return nil, offset, fmt.Errorf("bad length prefix %q: %v", lenHex, err)
		}
		if n < 4 {
			return nil, offset, fmt.Errorf("invalid outer pkt-line length %d", n)
		}
		end := offset + n
		if end > len(data) {
			return nil, offset, fmt.Errorf("outer pkt-line length %d exceeds buffer (offset=%d len=%d)", n, offset, len(data))
		}
		return data[offset+4 : end], end, nil
	}

	// --- First outer packet: should be sideband-wrapped "unpack ok\n" ---
	outerPayload, offset, err := parseOuterPktLine(raw, 0)
	if err != nil {
		t.Fatalf("parse first outer pkt-line: %v", err)
	}
	if len(outerPayload) == 0 {
		t.Fatal("first outer pkt-line payload is empty")
	}
	// First byte of outer payload is the sideband band number.
	if outerPayload[0] != 0x01 {
		t.Errorf("first outer pkt-line: want band byte 0x01, got 0x%02x (%q)", outerPayload[0], outerPayload[0:1])
	}
	// Remaining bytes are the inner pkt-line (including its own 4-hex length prefix).
	inner := string(outerPayload[1:])
	if !strings.Contains(inner, "unpack ok") {
		t.Errorf("first outer pkt-line: inner payload should contain 'unpack ok', got:\n%s\n(hex: %s)",
			inner, hex.EncodeToString([]byte(inner)))
	}

	// --- Second outer packet: should be sideband-wrapped "ng <ref> <reason>\n" ---
	outerPayload2, offset, err := parseOuterPktLine(raw, offset)
	if err != nil {
		t.Fatalf("parse second outer pkt-line: %v", err)
	}
	if len(outerPayload2) == 0 {
		t.Fatal("second outer pkt-line payload is empty")
	}
	if outerPayload2[0] != 0x01 {
		t.Errorf("second outer pkt-line: want band byte 0x01, got 0x%02x", outerPayload2[0])
	}
	inner2 := string(outerPayload2[1:])
	if !strings.Contains(inner2, "ng "+ref) {
		t.Errorf("second outer pkt-line: inner payload should contain 'ng %s', got:\n%s", ref, inner2)
	}

	// --- Final packet must be a flush (0000) ---
	if offset+4 > len(raw) {
		t.Fatalf("no bytes remaining for final flush packet at offset %d (len=%d)", offset, len(raw))
	}
	finalPkt := string(raw[offset : offset+4])
	if finalPkt != "0000" {
		t.Errorf("final packet: want '0000' flush, got %q", finalPkt)
	}
	if offset+4 != len(raw) {
		t.Errorf("unexpected trailing bytes after flush at offset %d (total len=%d)", offset+4, len(raw))
	}
}
