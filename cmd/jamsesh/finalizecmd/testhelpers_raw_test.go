package finalizecmd

import "os/exec"

// newRawCmd returns an *exec.Cmd for `git <args>` rooted in cwd, with
// no stdout/stderr wiring. Used by tests that want to run git without
// affecting (or being affected by) the package-level runGit overrides.
func newRawCmd(cwd string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	return cmd
}
