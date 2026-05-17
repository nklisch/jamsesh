// Package finalizecmd implements the "finalize" and "finalize-run"
// subcommands of the jamsesh CLI. They drive the local end of the
// finalize flow: opening the portal's curation view (or fetching the
// plan headlessly), then orchestrating the cherry-pick that produces
// the final branch in the user's source-repo checkout.
//
// All git invocations go through package-level function vars so unit
// tests can swap them out without process-spawning a real git binary.
// The real-git integration tests (preflight, execute, midpick) use the
// production helpers against a tempdir-backed `git init` repo.
package finalizecmd

import (
	"os"
	"os/exec"
	"strings"
)

// runGit invokes `git <args>` with stdout/stderr inherited. Used for
// every state-mutating git call where the user benefits from seeing
// real-time git output.
var runGit = func(args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runGitOutput is like runGit but captures stdout (trimmed). Stderr is
// inherited so error output still reaches the user.
var runGitOutput = func(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// runGitCwd runs `git <args>` with a working directory override. Used
// for the mid-pick detection check that needs to inspect the user's
// cwd before any state is mutated. Defaults to "." in production;
// tests inject a tempdir via the runGitOutputCwd var.
var runGitCwd = func(cwd string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runGitOutputCwd is the capturing variant of runGitCwd.
var runGitOutputCwd = func(cwd string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// runGitWithStdin executes `git <args>` while piping stdinData through
// the subprocess's stdin. Used for `git commit -F -` so the composed
// squash message reaches git without any shell-quoting risk.
var runGitWithStdin = func(stdinData string, args ...string) error {
	cmd := exec.Command("git", args...)
	cmd.Stdin = strings.NewReader(stdinData)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runGitCombined executes `git <args>` and returns the combined
// stdout/stderr output along with the exec error. Used for the
// idempotent-cleanup paths (e.g. `git remote remove jamsesh`) where
// the caller needs to classify the failure mode by inspecting stderr
// without spewing it onto the user's terminal — git prints
// "error: No such remote: 'jamsesh'" on stderr, which is benign in
// the cleanup context but alarming in the user's scrollback if we
// inherit stderr.
var runGitCombined = func(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

