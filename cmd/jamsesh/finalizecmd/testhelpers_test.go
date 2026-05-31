package finalizecmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitInTempRepo wraps git invocations to a tempdir-rooted repository.
// Returns the repo path. Tests use this to drive the real-git
// integration paths via runGit/runGitOutput overrides that pin the
// working dir.
func gitInTempRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustGitCwd(t, dir, "init", "-b", "main")
	mustGitCwd(t, dir, "config", "user.name", "Test User")
	mustGitCwd(t, dir, "config", "user.email", "test@example.com")
	mustGitCwd(t, dir, "config", "commit.gpgsign", "false")
	return dir
}

// mustGitCwd runs `git <args>` in cwd and fatals on error. The output
// is discarded; tests that need stdout/stderr should call exec
// directly.
func mustGitCwd(t *testing.T, cwd string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s in %s failed: %v\n%s", strings.Join(args, " "), cwd, err, out)
	}
}

// gitOutputCwd is like mustGitCwd but returns stdout.
func gitOutputCwd(t *testing.T, cwd string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s in %s failed: %v", strings.Join(args, " "), cwd, err)
	}
	return strings.TrimSpace(string(out))
}

// writeFile writes content to <dir>/<rel> and creates intermediate
// directories as needed.
func writeFile(t *testing.T, dir, rel, content string) {
	t.Helper()
	full := filepath.Join(dir, rel)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

// commit stages <rel> with content and commits with a message.
// Returns the new commit SHA.
func commit(t *testing.T, repo, rel, content, message string) string {
	t.Helper()
	writeFile(t, repo, rel, content)
	mustGitCwd(t, repo, "add", rel)
	mustGitCwd(t, repo, "commit", "-m", message)
	return gitOutputCwd(t, repo, "rev-parse", "HEAD")
}

// pinGitToCwd swaps the package-level runGit/runGitOutput vars to
// always pin the subprocess working dir to cwd. The original vars are
// restored via t.Cleanup. Use this from tests that want the
// production-shape `git` invocations (no Dir field on cmd.exec
// otherwise) but rooted in a tempdir instead of the test runner's
// actual working directory.
func pinGitToCwd(t *testing.T, cwd string) {
	t.Helper()
	oldRunGit := runGit
	oldRunGitWithEnv := runGitWithEnv
	oldRunGitOutput := runGitOutput
	oldRunGitWithStdin := runGitWithStdin
	oldRunGitCwd := runGitCwd
	oldRunGitOutputCwd := runGitOutputCwd
	oldRunGitCombined := runGitCombined
	t.Cleanup(func() {
		runGit = oldRunGit
		runGitWithEnv = oldRunGitWithEnv
		runGitOutput = oldRunGitOutput
		runGitWithStdin = oldRunGitWithStdin
		runGitCwd = oldRunGitCwd
		runGitOutputCwd = oldRunGitOutputCwd
		runGitCombined = oldRunGitCombined
	})

	runGit = func(args ...string) error {
		cmd := exec.Command("git", args...)
		cmd.Dir = cwd
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	runGitWithEnv = func(env []string, args ...string) error {
		cmd := exec.Command("git", args...)
		cmd.Dir = cwd
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if len(env) > 0 {
			cmd.Env = append(os.Environ(), env...)
		}
		return cmd.Run()
	}
	runGitOutput = func(args ...string) (string, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir = cwd
		cmd.Stderr = os.Stderr
		out, err := cmd.Output()
		return strings.TrimSpace(string(out)), err
	}
	runGitWithStdin = func(stdinData string, args ...string) error {
		cmd := exec.Command("git", args...)
		cmd.Dir = cwd
		cmd.Stdin = strings.NewReader(stdinData)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	runGitCwd = func(_ string, args ...string) error {
		cmd := exec.Command("git", args...)
		cmd.Dir = cwd
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		return cmd.Run()
	}
	runGitOutputCwd = func(_ string, args ...string) (string, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir = cwd
		cmd.Stderr = os.Stderr
		out, err := cmd.Output()
		return strings.TrimSpace(string(out)), err
	}
	runGitCombined = func(args ...string) (string, error) {
		cmd := exec.Command("git", args...)
		cmd.Dir = cwd
		out, err := cmd.CombinedOutput()
		return string(out), err
	}
}

// detectMidPickAt is a test-shim wrapper so tests can name an explicit
// cwd while production code uses the cwd-bound runGitOutputCwd var.
func detectMidPickAt(t *testing.T, cwd string) string {
	t.Helper()
	sha, err := detectMidPick(cwd)
	if err != nil {
		t.Fatalf("detectMidPick: %v", err)
	}
	return sha
}
