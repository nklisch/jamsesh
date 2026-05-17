package automerger

import (
	"strings"
)

// ParseConflictRanges scans content (the output of git merge-file --stdout)
// for conflict markers and returns the 1-indexed line ranges of each conflict
// region.  A conflict region spans from the "<<<<<<<" line through the
// ">>>>>>>" line (inclusive).
//
// The returned slice is empty when no conflict markers are found.
func ParseConflictRanges(content []byte) []LineRange {
	lines := strings.Split(string(content), "\n")
	// Remove the trailing empty element produced when content ends with "\n".
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	var ranges []LineRange
	inConflict := false
	startLine := 0

	for i, line := range lines {
		lineNum := i + 1 // 1-indexed
		switch {
		case strings.HasPrefix(line, "<<<<<<<"):
			inConflict = true
			startLine = lineNum
		case strings.HasPrefix(line, ">>>>>>>") && inConflict:
			ranges = append(ranges, LineRange{Start: startLine, End: lineNum})
			inConflict = false
			startLine = 0
		}
	}

	// Unterminated conflict block — treat remainder of file as conflicted.
	if inConflict {
		ranges = append(ranges, LineRange{Start: startLine, End: len(lines)})
	}

	return ranges
}
