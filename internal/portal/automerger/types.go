package automerger

// ResultKind classifies the outcome of a three-way merge.
type ResultKind string

const (
	// CleanMerge means the three-way merge succeeded with no conflicts.
	CleanMerge ResultKind = "clean-merge"
	// SafeAutoResolve means conflicts were present but all were
	// deterministically resolvable under a safe heuristic.
	// (Populated by story 2 — safe-auto-resolve.)
	SafeAutoResolve ResultKind = "safe-auto-resolve"
	// HardConflict means at least one conflict requires human judgment.
	HardConflict ResultKind = "hard-conflict"
)

// MergeResult is the output of [Merge]. The caller (auto-merger worker /
// outcomes feature) inspects Kind and acts accordingly:
//   - CleanMerge / SafeAutoResolve → create merge commit, advance draft ref.
//   - HardConflict → emit conflict.detected event with Conflicts payload.
type MergeResult struct {
	// Kind classifies the overall merge outcome.
	Kind ResultKind

	// MergedTreeSHA is the hex SHA of the root tree object that contains the
	// merged file tree. Populated for CleanMerge and SafeAutoResolve;
	// empty for HardConflict.
	MergedTreeSHA string

	// Heuristic is the safe-auto-resolve label ("whitespace" | "additions" |
	// "identical"). Non-empty only when Kind == SafeAutoResolve. When multiple
	// heuristics apply across files the most conservative label is used.
	Heuristic string

	// Conflicts holds the per-file conflict detail. Populated only when
	// Kind == HardConflict.
	Conflicts []Conflict
}

// Conflict describes a conflicted file and the line ranges of each conflict
// region within it.
type Conflict struct {
	// File is the repo-relative path of the conflicted file.
	File string
	// Ranges lists the 1-indexed [Start, End] line ranges (inclusive) of each
	// conflict region in the file.
	Ranges []LineRange
}

// LineRange is a 1-indexed, inclusive line range within a file.
type LineRange struct {
	Start int // first line of the conflict region (1-indexed)
	End   int // last line of the conflict region (1-indexed, inclusive)
}
