package automerger

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// tryAutoResolve attempts to classify the three-way conflict between base,
// ours, and theirs under one of the three locked safe heuristics. Detection
// is performed in conservative-first order:
//
//  1. identical — both sides made the same bytewise change
//  2. whitespace — differences are only trailing whitespace / line endings
//  3. additions  — both sides only added lines (no modify, no delete) and
//     the added content is not identical between the two sides
//
// Returns (resolved, heuristic, true) when a safe resolution is found, or
// (nil, "", false) when the conflict must escalate to HardConflict.
//
// Binary files are never auto-resolved.
func tryAutoResolve(base, ours, theirs []byte) (resolved []byte, heuristic string, ok bool) {
	if isBinary(base) || isBinary(ours) || isBinary(theirs) {
		return nil, "", false
	}

	if r, ok := isIdenticalEdit(base, ours, theirs); ok {
		return r, "identical", true
	}
	if r, ok := isWhitespaceOnly(base, ours, theirs); ok {
		return r, "whitespace", true
	}
	if r, ok := isNonOverlappingAddition(base, ours, theirs); ok {
		return r, "additions", true
	}
	return nil, "", false
}

// isBinary returns true when the first 8000 bytes of b contain a null byte —
// the same heuristic used by git to classify binary files.
func isBinary(b []byte) bool {
	n := len(b)
	if n > 8000 {
		n = 8000
	}
	return bytes.IndexByte(b[:n], 0) >= 0
}

// isIdenticalEdit returns (ours, true) iff ours == theirs (byte-exact) AND
// ours != base (i.e., both sides made the same non-trivial change).
func isIdenticalEdit(base, ours, theirs []byte) (resolved []byte, ok bool) {
	if bytes.Equal(ours, theirs) && !bytes.Equal(ours, base) {
		return ours, true
	}
	return nil, false
}

// isWhitespaceOnly returns (ours, true) iff, after normalising line endings
// to LF and trimming trailing spaces/tabs from each line, the content of
// ours and theirs produce the same line set as base.
//
// Indentation-depth changes are NOT permitted: if the number of leading tabs
// on any line changes between base and either side, the heuristic returns
// false to escalate to HardConflict (tab-count is a proxy for indent depth).
func isWhitespaceOnly(base, ours, theirs []byte) (resolved []byte, ok bool) {
	baseLF := normaliseLF(base)
	oursLF := normaliseLF(ours)
	theirsLF := normaliseLF(theirs)

	baseLines := splitLines(baseLF)
	oursLines := splitLines(oursLF)
	theirsLines := splitLines(theirsLF)

	// Line counts must match after normalisation; a whitespace-only change
	// cannot add or remove logical lines.
	if len(oursLines) != len(baseLines) || len(theirsLines) != len(baseLines) {
		return nil, false
	}

	for i, baseLine := range baseLines {
		baseStripped := strings.TrimRight(baseLine, " \t")
		oursStripped := strings.TrimRight(oursLines[i], " \t")
		theirsStripped := strings.TrimRight(theirsLines[i], " \t")

		if oursStripped != baseStripped || theirsStripped != baseStripped {
			return nil, false
		}

		// Guard: indentation depth (leading tab count) must not change.
		if leadingTabs(oursLines[i]) != leadingTabs(baseLine) {
			return nil, false
		}
		if leadingTabs(theirsLines[i]) != leadingTabs(baseLine) {
			return nil, false
		}
	}

	return ours, true
}

// isNonOverlappingAddition returns (merged, true) iff both sides ONLY add
// lines relative to base (no MODIFY, no DELETE), and the added lines are not
// identical between the two sides (duplicate-add escalates to HardConflict).
//
// When safe, the resolution interleaves added lines in ascending line-number
// order (ours first when both add at the same position).
func isNonOverlappingAddition(base, ours, theirs []byte) (resolved []byte, ok bool) {
	oursHunks, err := diffAddOnly(base, ours)
	if err != nil {
		return nil, false
	}
	theirHunks, err := diffAddOnly(base, theirs)
	if err != nil {
		return nil, false
	}

	// Both sides must be pure-add; nil means "not pure add".
	if oursHunks == nil || theirHunks == nil {
		return nil, false
	}

	// Collect all added lines from both sides.
	type addedLine struct {
		afterLine int    // 0-indexed base line after which to insert
		content   string // the added line content (without trailing \n)
		side      int    // 0 = ours, 1 = theirs
	}

	var allAdded []addedLine

	oursAddedSet := make(map[string]struct{})
	for _, h := range oursHunks {
		for _, line := range h.addedLines {
			oursAddedSet[line] = struct{}{}
			allAdded = append(allAdded, addedLine{afterLine: h.afterBaseLine, content: line, side: 0})
		}
	}
	for _, h := range theirHunks {
		for _, line := range h.addedLines {
			if _, dup := oursAddedSet[line]; dup {
				// Duplicate-add safety: escalate.
				return nil, false
			}
			allAdded = append(allAdded, addedLine{afterLine: h.afterBaseLine, content: line, side: 1})
		}
	}

	// Sort: ascending afterLine; ties broken with ours (0) before theirs (1).
	sort.SliceStable(allAdded, func(i, j int) bool {
		if allAdded[i].afterLine != allAdded[j].afterLine {
			return allAdded[i].afterLine < allAdded[j].afterLine
		}
		return allAdded[i].side < allAdded[j].side
	})

	// Reconstruct the merged content by walking base lines and inserting
	// added lines at the right positions.
	baseLines := splitLines(normaliseLF(base))
	var buf bytes.Buffer

	addIdx := 0
	// Insert additions that come BEFORE base line 0 (afterLine == -1 or 0 convention).
	// Our afterBaseLine uses 0-indexed "after base line i" semantics:
	//   afterLine = -1 means "before first base line"
	//   afterLine = N  means "after base line N" (0-indexed)

	for i, bl := range baseLines {
		// Insert lines to be added BEFORE base line i: these have afterLine == i-1
		// (they come after the previous base line, i.e. before the current one).
		for addIdx < len(allAdded) && allAdded[addIdx].afterLine == i-1 {
			buf.WriteString(allAdded[addIdx].content)
			buf.WriteByte('\n')
			addIdx++
		}
		buf.WriteString(bl)
		buf.WriteByte('\n')
	}
	// Flush remaining additions (after last base line).
	for addIdx < len(allAdded) {
		buf.WriteString(allAdded[addIdx].content)
		buf.WriteByte('\n')
		addIdx++
	}

	return buf.Bytes(), true
}

// addHunk records a batch of lines added at a specific position relative to
// base, expressed as "after this 0-indexed base line" (-1 = prepend before
// everything).
type addHunk struct {
	afterBaseLine int      // -1 = before first line; N = after base line N
	addedLines    []string // the added line contents (without trailing newline)
}

// classifyDiffErr classifies a diff subprocess error into acceptable (nil) or
// a real failure. diff exit codes:
//
//   - 0: files are identical
//   - 1: files differ (expected; not an error)
//   - 2+: trouble (missing binary, unreadable file, etc.) — returned as an error
//
// A non-*exec.ExitError (e.g. diff not on PATH) is also returned as an error.
func classifyDiffErr(err error) error {
	if err == nil {
		return nil
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		// e.g. exec.ErrNotFound — diff binary not on PATH
		return fmt.Errorf("diff subprocess: %w", err)
	}
	code := exitErr.ExitCode()
	if code == 0 || code == 1 {
		// 0 = identical, 1 = differences — both are expected.
		return nil
	}
	return fmt.Errorf("diff subprocess exited with code %d: %w", code, err)
}

// runDiff runs `diff -u baseFile otherFile` and returns its stdout.
// Exit codes 0 (identical) and 1 (differences) are treated as success.
// Exit code >= 2 or a non-*exec.ExitError is returned as an error.
func runDiff(baseFile, otherFile string) ([]byte, error) {
	cmd := exec.Command("diff", "-u", baseFile, otherFile)
	var out bytes.Buffer
	cmd.Stdout = &out
	err := cmd.Run()
	if err := classifyDiffErr(err); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// diffAddOnly runs `diff -u base other` and parses the unified diff.
// Returns a slice of addHunk if and only if ALL changes are pure additions
// (no deletions or modifications), or nil if any deletion/modification is
// found.
//
// An error from the diff subprocess is returned as (nil, err); a nil slice
// with nil error means "not pure add".
func diffAddOnly(base, other []byte) ([]addHunk, error) {
	if bytes.Equal(base, other) {
		return []addHunk{}, nil // no changes at all
	}

	dir, err := os.MkdirTemp("", "jamsesh-diff-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	baseFile := filepath.Join(dir, "base")
	otherFile := filepath.Join(dir, "other")
	if err := os.WriteFile(baseFile, base, 0o600); err != nil {
		return nil, err
	}
	if err := os.WriteFile(otherFile, other, 0o600); err != nil {
		return nil, err
	}

	diffOut, err := runDiff(baseFile, otherFile)
	if err != nil {
		return nil, err
	}

	return parseUnifiedDiffAddOnly(diffOut)
}

// parseUnifiedDiffAddOnly parses a unified diff and returns addHunks when
// all changes are pure additions. Returns nil (not error) when any deletion
// or modification is found.
func parseUnifiedDiffAddOnly(diff []byte) ([]addHunk, error) {
	var hunks []addHunk
	var current *addHunk

	scanner := bufio.NewScanner(bytes.NewReader(diff))
	baseLineNo := -1 // current 0-indexed base line cursor (within hunk)

	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++"):
			// Header lines — skip.
			continue

		case strings.HasPrefix(line, "@@ "):
			// @@ -oldStart,oldLen +newStart,newLen @@
			// Parse the old (base) range to determine where we are.
			oldStart, _, err := parseHunkHeader(line)
			if err != nil {
				return nil, err
			}
			// oldStart is 1-indexed; convert to 0-indexed.
			// The hunk starts "at" base line oldStart-1.
			// We track afterBaseLine as the last base line we've seen
			// before any additions.
			baseLineNo = oldStart - 2 // one before first old line (so first context/old bumps to oldStart-1)
			current = &addHunk{afterBaseLine: baseLineNo}

		case strings.HasPrefix(line, "-"):
			// Deletion or modification — not pure add.
			return nil, nil

		case strings.HasPrefix(line, "+"):
			// Pure addition.
			if current == nil {
				// Diff output before any @@ header — ignore.
				continue
			}
			current.afterBaseLine = baseLineNo
			current.addedLines = append(current.addedLines, line[1:])

		default:
			// Context line — advance base line cursor.
			if current != nil {
				baseLineNo++
				// When we see a context line after additions, flush the current hunk
				// and start a new one for potential subsequent additions.
				if len(current.addedLines) > 0 {
					hunks = append(hunks, *current)
					current = &addHunk{afterBaseLine: baseLineNo}
				} else {
					current.afterBaseLine = baseLineNo
				}
			}
		}
	}

	if current != nil && len(current.addedLines) > 0 {
		hunks = append(hunks, *current)
	}

	return hunks, nil
}

// parseHunkHeader extracts (oldStart, oldLen) from a unified-diff @@ line.
// Format: @@ -oldStart[,oldLen] +newStart[,newLen] @@
func parseHunkHeader(line string) (oldStart, oldLen int, err error) {
	// Find the "-" range: between "@@ -" and the next " +"
	start := strings.Index(line, "@@ -")
	if start < 0 {
		return 0, 0, nil
	}
	rest := line[start+4:]
	end := strings.Index(rest, " +")
	if end < 0 {
		end = len(rest)
	}
	oldRange := rest[:end]
	// oldRange is either "N" or "N,M"
	if comma := strings.Index(oldRange, ","); comma >= 0 {
		oldStart, err = strconv.Atoi(oldRange[:comma])
		if err != nil {
			return 0, 0, err
		}
		oldLen, err = strconv.Atoi(oldRange[comma+1:])
		if err != nil {
			return 0, 0, err
		}
	} else {
		oldStart, err = strconv.Atoi(oldRange)
		if err != nil {
			return 0, 0, err
		}
		oldLen = 1
	}
	return oldStart, oldLen, nil
}

// normaliseLF replaces all CRLF sequences with LF.
func normaliseLF(b []byte) []byte {
	return bytes.ReplaceAll(b, []byte("\r\n"), []byte("\n"))
}

// splitLines splits b on "\n". A trailing newline produces no trailing empty
// element (it is consumed).
func splitLines(b []byte) []string {
	if len(b) == 0 {
		return nil
	}
	s := string(b)
	if strings.HasSuffix(s, "\n") {
		s = s[:len(s)-1]
	}
	return strings.Split(s, "\n")
}

// leadingTabs counts the number of leading tab characters in s.
func leadingTabs(s string) int {
	n := 0
	for _, c := range s {
		if c == '\t' {
			n++
		} else {
			break
		}
	}
	return n
}

// heuristicPriority returns a numeric priority for a heuristic name so that
// the most conservative (least risky) wins when multiple apply across files.
// Lower numbers are more conservative.
func heuristicPriority(h string) int {
	switch h {
	case "identical":
		return 0
	case "whitespace":
		return 1
	case "additions":
		return 2
	default:
		return 99
	}
}
