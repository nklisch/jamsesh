// Package checkout provides a local "source repo" sandbox for running
// finalize plan bodies in e2e tests. The Sandbox is a temporary git
// repository that can execute the shell script the portal emits and then
// expose the resulting commit history for assertions.
//
// No external forges or network access are used; this is a fully local
// tmpdir repo. The plan body's `$JAMSESH_FETCH_REMOTE` placeholder is
// substituted with the sandbox directory itself (or another local path)
// by the caller before handing the script to RunPlan.
package checkout

import (
	"os/exec"
	"strings"
	"testing"
)

// Sandbox is a local git repository that can run finalize plan bodies.
type Sandbox struct {
	// Dir is the root of the tmpdir repository.
	Dir string
}

// Start creates a fresh tmpdir, runs `git init -b main`, configures a
// git identity so commits don't fail, and returns the Sandbox.
func Start(t *testing.T) *Sandbox {
	t.Helper()
	dir := t.TempDir()

	run(t, dir, "git", "init", "-q", "-b", "main")
	// Set a stable identity so `git commit` never prompts.
	run(t, dir, "git", "config", "user.email", "finalize-runner@test.example")
	run(t, dir, "git", "config", "user.name", "Finalize Runner")

	return &Sandbox{Dir: dir}
}

// RunPlan executes the given plan body as a shell script inside the
// sandbox directory. The environment variable JAMSESH_FETCH_REMOTE is
// set to fetchRemote (use the source repo path for a fully local run).
// JAMSESH_RUNNER_NAME and JAMSESH_RUNNER_EMAIL are set to stable test
// values so `git commit --author` works without a configured identity.
//
// When fetchToken is non-empty, git's one-shot config mechanism is used
// to inject `http.extraHeader=Authorization: Bearer <token>` for the
// duration of the script, matching the plugin's credential-passing
// strategy (token never stored in .git/config).
//
// Returns combined stdout+stderr for debugging. The test is failed
// immediately if the script exits non-zero.
func (s *Sandbox) RunPlan(t *testing.T, planBody, fetchRemote, fetchToken string) string {
	t.Helper()
	// The generated plan starts with '#!/usr/bin/env bash' and uses
	// 'set -o pipefail'. /bin/sh is dash on debian-based hosts and dash
	// doesn't support pipefail — running the script under sh yields
	// '/bin/sh: 2: set: Illegal option -o pipefail'. Use bash explicitly
	// so the shell matches the script's shebang.
	cmd := exec.Command("/bin/bash", "-c", planBody)
	cmd.Dir = s.Dir
	env := append(sandboxEnv(s.Dir),
		"JAMSESH_FETCH_REMOTE="+fetchRemote,
		"JAMSESH_RUNNER_NAME=Finalize Runner",
		"JAMSESH_RUNNER_EMAIL=finalize-runner@test.example",
	)
	if fetchToken != "" {
		// Inject the bearer token as a one-shot git config via environment
		// so the script's `git fetch "$JAMSESH_FETCH_REMOTE"` can
		// authenticate without embedding the token in the URL.
		env = append(env,
			"GIT_CONFIG_COUNT=1",
			"GIT_CONFIG_KEY_0=http.extraHeader",
			"GIT_CONFIG_VALUE_0=Authorization: Bearer "+fetchToken,
		)
	}
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("checkout.RunPlan: script failed: %v\noutput:\n%s", err, out)
	}
	return string(out)
}

// Log returns the output of `git log --pretty=format:%H%n%B%n---` for
// the given ref. The format includes the full SHA, the full commit
// message (body included), and a `---` separator per commit. Pass an
// empty ref to use HEAD.
func (s *Sandbox) Log(t *testing.T, ref string) string {
	t.Helper()
	if ref == "" {
		ref = "HEAD"
	}
	return runOutput(t, s.Dir, "git", "log", ref, "--pretty=format:%H%n%B%n---")
}

// Branch returns the current branch name (git branch --show-current).
func (s *Sandbox) Branch(t *testing.T) string {
	t.Helper()
	return strings.TrimSpace(runOutput(t, s.Dir, "git", "branch", "--show-current"))
}

// CommitCount returns the number of commits reachable from ref.
func (s *Sandbox) CommitCount(t *testing.T, ref string) int {
	t.Helper()
	if ref == "" {
		ref = "HEAD"
	}
	out := strings.TrimSpace(runOutput(t, s.Dir, "git", "rev-list", "--count", ref))
	count := 0
	for _, c := range out {
		if c >= '0' && c <= '9' {
			count = count*10 + int(c-'0')
		}
	}
	return count
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// sandboxEnv returns a minimal environment suitable for running git
// commands in the sandbox: HOME, PATH, and GIT_CONFIG_NOSYSTEM are set
// to isolate the sandbox from the test host's git configuration.
func sandboxEnv(dir string) []string {
	return []string{
		"HOME=" + dir,
		"PATH=/usr/bin:/bin:/usr/local/bin",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=Finalize Runner",
		"GIT_AUTHOR_EMAIL=finalize-runner@test.example",
		"GIT_COMMITTER_NAME=Finalize Runner",
		"GIT_COMMITTER_EMAIL=finalize-runner@test.example",
	}
}

// run executes a command in dir, failing the test on any error.
func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = sandboxEnv(dir)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("checkout: %s %s: %v\n%s", name, strings.Join(args, " "), err, out)
	}
}

// runOutput executes a command in dir and returns its stdout.
func runOutput(t *testing.T, dir string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		var stderr []byte
		if ee, ok := err.(*exec.ExitError); ok {
			stderr = ee.Stderr
		}
		t.Fatalf("checkout: %s %s: %v\n%s", name, strings.Join(args, " "), err, stderr)
	}
	return string(out)
}
