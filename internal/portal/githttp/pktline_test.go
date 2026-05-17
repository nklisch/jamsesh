package githttp

import (
	"bytes"
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
func TestReadCommandList_SingleUpdate(t *testing.T) {
	oldSHA := strings.Repeat("a", 40)
	newSHA := strings.Repeat("b", 40)
	ref := "refs/heads/jam/sess-1/acc-1/main"
	packData := []byte("PACK\x00\x00\x00\x02")

	raw := buildTestCommandList([]prereceive.RefUpdate{
		{OldSHA: oldSHA, NewSHA: newSHA, Ref: ref},
	}, packData)

	updates, packReader, err := readCommandList(bytes.NewReader(raw))
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

	got, _, err := readCommandList(bytes.NewReader(raw))
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

	updates, _, err := readCommandList(bytes.NewReader(raw))
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
	updates, _, err := readCommandList(bytes.NewReader(raw))
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
	_, _, err := readCommandList(bytes.NewReader(raw))
	if err == nil {
		t.Error("expected error for invalid hex prefix, got nil")
	}
}

// TestWriteReportStatusRejection_AllNg verifies that all updates are
// written as "ng" lines.
func TestWriteReportStatusRejection_AllNg(t *testing.T) {
	updates := []prereceive.RefUpdate{
		{Ref: "refs/heads/jam/sess-1/acc-1/main", OldSHA: strings.Repeat("a", 40), NewSHA: strings.Repeat("b", 40)},
		{Ref: "refs/heads/jam/sess-1/acc-1/feat", OldSHA: strings.Repeat("b", 40), NewSHA: strings.Repeat("c", 40)},
	}
	rejections := []prereceive.Rejection{
		{Code: prereceive.CodeMissingTrailer, Message: "missing required trailers", Details: map[string]any{}},
	}

	var buf bytes.Buffer
	writeReportStatusRejection(&buf, updates, rejections)

	out := buf.String()

	// Must start with unpack ok pkt-line.
	if !strings.Contains(out, "unpack ok") {
		t.Error("report-status should contain 'unpack ok'")
	}

	// Both refs must have "ng" lines.
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
	writeReportStatusRejection(&buf, updates, rejections)

	out := buf.String()
	if !strings.Contains(out, "ref namespace violation") {
		t.Errorf("expected per-ref reason in output, got:\n%s", out)
	}
}
