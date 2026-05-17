package automerger

import (
	"bytes"
	"testing"
)

// ---------------------------------------------------------------------------
// isBinary
// ---------------------------------------------------------------------------

func TestIsBinary_NullByte(t *testing.T) {
	if !isBinary([]byte("hello\x00world")) {
		t.Error("expected isBinary to return true for content with null byte")
	}
}

func TestIsBinary_PlainText(t *testing.T) {
	if isBinary([]byte("hello world\n")) {
		t.Error("expected isBinary to return false for plain text")
	}
}

func TestIsBinary_Empty(t *testing.T) {
	if isBinary([]byte{}) {
		t.Error("expected isBinary to return false for empty slice")
	}
}

func TestIsBinary_NullBeyond8000(t *testing.T) {
	// Null byte placed beyond the 8000-byte window should not trigger detection.
	b := make([]byte, 9000)
	for i := range b {
		b[i] = 'a'
	}
	b[8001] = 0
	if isBinary(b) {
		t.Error("expected isBinary to return false when null byte is beyond 8000-byte window")
	}
}

// ---------------------------------------------------------------------------
// isIdenticalEdit
// ---------------------------------------------------------------------------

func TestIsIdenticalEdit_BothSameDifferentFromBase(t *testing.T) {
	base := []byte("original\n")
	change := []byte("modified\n")
	r, ok := isIdenticalEdit(base, change, change)
	if !ok {
		t.Fatal("expected ok=true for identical edits")
	}
	if !bytes.Equal(r, change) {
		t.Errorf("resolved: got %q, want %q", r, change)
	}
}

func TestIsIdenticalEdit_SameAsBase(t *testing.T) {
	base := []byte("original\n")
	_, ok := isIdenticalEdit(base, base, base)
	if ok {
		t.Error("expected ok=false when ours == theirs == base (no actual change)")
	}
}

func TestIsIdenticalEdit_DifferentFromEachOther(t *testing.T) {
	base := []byte("original\n")
	ours := []byte("our version\n")
	theirs := []byte("their version\n")
	_, ok := isIdenticalEdit(base, ours, theirs)
	if ok {
		t.Error("expected ok=false when ours != theirs")
	}
}

func TestIsIdenticalEdit_OursSameAsBase(t *testing.T) {
	base := []byte("original\n")
	theirs := []byte("modified\n")
	_, ok := isIdenticalEdit(base, base, theirs)
	if ok {
		t.Error("expected ok=false when ours == base but theirs != base")
	}
}

// ---------------------------------------------------------------------------
// isWhitespaceOnly
// ---------------------------------------------------------------------------

func TestIsWhitespaceOnly_TrailingSpaces(t *testing.T) {
	base := []byte("line one   \nline two  \n")
	ours := []byte("line one\nline two\n")
	theirs := []byte("line one\nline two\n")
	r, ok := isWhitespaceOnly(base, ours, theirs)
	if !ok {
		t.Fatal("expected ok=true for trailing-whitespace-only change")
	}
	if !bytes.Equal(r, ours) {
		t.Errorf("resolved should equal ours; got %q, want %q", r, ours)
	}
}

func TestIsWhitespaceOnly_NoChange(t *testing.T) {
	same := []byte("line one\nline two\n")
	_, ok := isWhitespaceOnly(same, same, same)
	// Same content on all three — technically whitespace-only (no change),
	// but isIdenticalEdit would catch this first in tryAutoResolve.
	// isWhitespaceOnly should still return true here.
	if !ok {
		t.Error("expected ok=true when all three are identical")
	}
}

func TestIsWhitespaceOnly_LineMismatch(t *testing.T) {
	base := []byte("line one\nline two\n")
	ours := []byte("line one MODIFIED\nline two\n")
	theirs := []byte("line one\nline two\n")
	_, ok := isWhitespaceOnly(base, ours, theirs)
	if ok {
		t.Error("expected ok=false when ours modifies actual content")
	}
}

func TestIsWhitespaceOnly_DifferentLineCount(t *testing.T) {
	base := []byte("line one\nline two\n")
	ours := []byte("line one\nline two\nline three\n") // extra line
	theirs := []byte("line one\nline two\n")
	_, ok := isWhitespaceOnly(base, ours, theirs)
	if ok {
		t.Error("expected ok=false when line count differs")
	}
}

func TestIsWhitespaceOnly_CRLFNormalisation(t *testing.T) {
	base := []byte("line one\r\nline two\r\n")
	ours := []byte("line one\nline two\n")   // LF only
	theirs := []byte("line one\nline two\n") // LF only
	r, ok := isWhitespaceOnly(base, ours, theirs)
	if !ok {
		t.Fatal("expected ok=true for CRLF→LF normalisation change")
	}
	if !bytes.Equal(r, ours) {
		t.Errorf("resolved should equal ours; got %q", r)
	}
}

func TestIsWhitespaceOnly_IndentationDepthChange(t *testing.T) {
	// Base uses tabs; ours+theirs convert to spaces (different tab count).
	base := []byte("\tfunction() {\n\t\tvar x;\n\t}\n")
	ours := []byte("  function() {\n    var x;\n  }\n")
	theirs := []byte("  function() {\n    var x;\n  }\n")
	_, ok := isWhitespaceOnly(base, ours, theirs)
	if ok {
		t.Error("expected ok=false for indentation-depth change (tabs→spaces)")
	}
}

// ---------------------------------------------------------------------------
// isNonOverlappingAddition
// ---------------------------------------------------------------------------

func TestIsNonOverlappingAddition_DisjointAdds(t *testing.T) {
	base := []byte("middle\n")
	ours := []byte("ADDED TOP\nmiddle\n")
	theirs := []byte("middle\nADDED BOTTOM\n")
	r, ok := isNonOverlappingAddition(base, ours, theirs)
	if !ok {
		t.Fatal("expected ok=true for disjoint additions")
	}
	// Resolved must contain all three logical lines.
	s := string(r)
	for _, want := range []string{"ADDED TOP", "middle", "ADDED BOTTOM"} {
		if !bytes.Contains(r, []byte(want)) {
			t.Errorf("resolved missing %q; got:\n%s", want, s)
		}
	}
}

func TestIsNonOverlappingAddition_SharedLine(t *testing.T) {
	base := []byte("line one\nline two\n")
	ours := []byte("line one\nDUPLICATE\nline two\n")
	theirs := []byte("line one\nDUPLICATE\nline two\n")
	_, ok := isNonOverlappingAddition(base, ours, theirs)
	if ok {
		t.Error("expected ok=false when both sides add identical line content")
	}
}

func TestIsNonOverlappingAddition_ModifyNotAdd(t *testing.T) {
	base := []byte("line one\nline two\n")
	ours := []byte("MODIFIED ONE\nline two\n") // modification, not addition
	theirs := []byte("line one\nNEW LINE\nline two\n")
	_, ok := isNonOverlappingAddition(base, ours, theirs)
	if ok {
		t.Error("expected ok=false when one side modifies (not just adds)")
	}
}

func TestIsNonOverlappingAddition_DeleteNotAdd(t *testing.T) {
	base := []byte("line one\nline two\nline three\n")
	ours := []byte("line one\nline three\n") // deletion
	theirs := []byte("line one\nline two\nNEW LINE\nline three\n")
	_, ok := isNonOverlappingAddition(base, ours, theirs)
	if ok {
		t.Error("expected ok=false when one side deletes lines")
	}
}

func TestIsNonOverlappingAddition_NoChanges(t *testing.T) {
	same := []byte("line one\nline two\n")
	r, ok := isNonOverlappingAddition(same, same, same)
	if !ok {
		t.Fatal("expected ok=true when no changes (trivially disjoint)")
	}
	if !bytes.Equal(r, same) {
		t.Errorf("resolved should equal base when no changes; got %q", r)
	}
}

// ---------------------------------------------------------------------------
// tryAutoResolve dispatcher
// ---------------------------------------------------------------------------

func TestTryAutoResolve_BinaryEscalates(t *testing.T) {
	bin := []byte("data\x00binary")
	_, _, ok := tryAutoResolve(bin, bin, bin)
	if ok {
		t.Error("expected ok=false for binary inputs")
	}
}

func TestTryAutoResolve_IdenticalFirst(t *testing.T) {
	base := []byte("original\n")
	change := []byte("modified\n")
	_, h, ok := tryAutoResolve(base, change, change)
	if !ok {
		t.Fatal("expected ok=true for identical edit")
	}
	if h != "identical" {
		t.Errorf("heuristic: got %q, want %q", h, "identical")
	}
}

func TestTryAutoResolve_WhitespaceSecond(t *testing.T) {
	base := []byte("trailing   \n")
	clean := []byte("trailing\n")
	_, h, ok := tryAutoResolve(base, clean, clean)
	// ours == theirs == clean (both differ from base) → identical fires first.
	// To test whitespace specifically, make ours and theirs differ in whitespace.
	_ = ok
	_ = h
	// Different trailing whitespace on ours vs theirs but same stripped content.
	ours2 := []byte("trailing \n")
	theirs2 := []byte("trailing\n")
	_, h2, ok2 := tryAutoResolve(base, ours2, theirs2)
	if !ok2 {
		t.Fatal("expected ok=true for whitespace-only difference")
	}
	if h2 != "whitespace" {
		t.Errorf("heuristic: got %q, want %q", h2, "whitespace")
	}
}

func TestTryAutoResolve_AdditionsThird(t *testing.T) {
	base := []byte("middle\n")
	ours := []byte("TOP\nmiddle\n")
	theirs := []byte("middle\nBOTTOM\n")
	_, h, ok := tryAutoResolve(base, ours, theirs)
	if !ok {
		t.Fatal("expected ok=true for disjoint additions")
	}
	if h != "additions" {
		t.Errorf("heuristic: got %q, want %q", h, "additions")
	}
}

func TestTryAutoResolve_HardConflict(t *testing.T) {
	base := []byte("line\n")
	ours := []byte("OURS\n")
	theirs := []byte("THEIRS\n")
	_, _, ok := tryAutoResolve(base, ours, theirs)
	if ok {
		t.Error("expected ok=false for genuinely conflicting edits")
	}
}

// ---------------------------------------------------------------------------
// heuristicPriority
// ---------------------------------------------------------------------------

func TestHeuristicPriority_Order(t *testing.T) {
	if heuristicPriority("identical") >= heuristicPriority("whitespace") {
		t.Error("identical should have lower priority number than whitespace")
	}
	if heuristicPriority("whitespace") >= heuristicPriority("additions") {
		t.Error("whitespace should have lower priority number than additions")
	}
}
