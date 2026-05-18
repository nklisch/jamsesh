package finalize

import (
	"regexp"
	"strings"
)

// shellquote returns s wrapped in single quotes with any internal single
// quotes escaped via the '\'' trick. The output is safe to splice into a
// bash command whether the surrounding context is double-quoted, single-
// quoted, or unquoted.
//
// Examples:
//
//	shellquote("ship/comments")    → 'ship/comments'
//	shellquote("it's")             → 'it'\''s'
//	shellquote("")                 → ''
func shellquote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// Shellquote is the exported form of shellquote. It is provided so that the
// companion test story (gate-tests-finalize-shell-escape) can write
// negative-path assertions in the finalize_test package without re-
// implementing the quoting logic.
func Shellquote(s string) string { return shellquote(s) }

// reTargetBranch matches valid target branch names: non-empty, only
// alphanumeric characters plus dots, underscores, hyphens, and forward
// slashes, and must not start with a hyphen (which git interprets as a flag).
var reTargetBranch = regexp.MustCompile(`^[A-Za-z0-9._/][A-Za-z0-9._/-]*$`)

// reBaseSHA matches a full 40-hex-character SHA-1. Adjust to 64 if SHA-256
// is adopted elsewhere in the codebase.
var reBaseSHA = regexp.MustCompile(`^[a-f0-9]{40}$`)

// ValidateTargetBranch returns true when branch is a safe, non-empty branch
// name that passes the shape check and does not start with '-'.
//
// Exported so the companion test story can call it directly.
func ValidateTargetBranch(branch string) bool {
	if branch == "" {
		return false
	}
	if branch[0] == '-' {
		return false
	}
	return reTargetBranch.MatchString(branch)
}

// ValidateBaseSHA returns true when sha is a full 40-hex-digit SHA-1.
//
// Exported so the companion test story can call it directly.
func ValidateBaseSHA(sha string) bool {
	return reBaseSHA.MatchString(sha)
}
