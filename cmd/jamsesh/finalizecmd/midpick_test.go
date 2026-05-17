package finalizecmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestDetectMidPick_noPick(t *testing.T) {
	repo := gitInTempRepo(t)
	commit(t, repo, "a.txt", "one", "first")
	if sha := detectMidPickAt(t, repo); sha != "" {
		t.Errorf("expected empty SHA, got %q", sha)
	}
}

func TestDetectMidPick_notARepo(t *testing.T) {
	dir := t.TempDir()
	// not a git repo
	sha, err := detectMidPick(dir)
	if err != nil {
		t.Errorf("expected nil error for non-repo, got %v", err)
	}
	if sha != "" {
		t.Errorf("expected empty SHA for non-repo, got %q", sha)
	}
}

func TestDetectMidPick_inProgress(t *testing.T) {
	repo := gitInTempRepo(t)
	// Create a conflict: two branches modify the same line.
	commit(t, repo, "x.txt", "v1\n", "base")
	mustGitCwd(t, repo, "checkout", "-b", "branch-a")
	commit(t, repo, "x.txt", "v2\n", "branch a change")
	mustGitCwd(t, repo, "checkout", "main")
	conflictSHA := commit(t, repo, "x.txt", "v3\n", "main change")
	_ = conflictSHA

	// Cherry-pick branch-a's commit into main — will conflict.
	branchASHA := gitOutputCwd(t, repo, "rev-parse", "branch-a")
	cmdErr := func() error {
		// Use git directly so we don't pollute runGit.
		return runOnceCwd(repo, "cherry-pick", branchASHA)
	}()
	if cmdErr == nil {
		t.Fatalf("expected cherry-pick to conflict, got nil")
	}

	sha := detectMidPickAt(t, repo)
	if sha == "" {
		t.Fatalf("expected non-empty CHERRY_PICK_HEAD SHA")
	}
	if sha != branchASHA {
		t.Errorf("CHERRY_PICK_HEAD = %q, want %q", sha, branchASHA)
	}
}

func TestReportMidPick_withPlan(t *testing.T) {
	plan := &Plan{
		SelectedCommits: []PlanCommit{
			{SHA: "aaa1", Subject: "first"},
			{SHA: "bbb2", Subject: "second"},
			{SHA: "ccc3", Subject: "third"},
		},
	}
	var buf bytes.Buffer
	reportMidPick(&buf, "bbb2", plan)
	out := buf.String()
	for _, want := range []string{
		"Offending commit: bbb2",
		"ccc3  third",
		"git cherry-pick --continue",
		"git cherry-pick --abort",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "aaa1  first") {
		t.Errorf("output should not list commits before offending SHA: %s", out)
	}
}

func TestReportMidPick_noPlan(t *testing.T) {
	var buf bytes.Buffer
	reportMidPick(&buf, "deadbeef", nil)
	out := buf.String()
	if !strings.Contains(out, "Offending commit: deadbeef") {
		t.Errorf("missing offending SHA: %s", out)
	}
	if !strings.Contains(out, "git cherry-pick --continue") {
		t.Errorf("missing resume hint: %s", out)
	}
}

func TestRemainingAfter(t *testing.T) {
	commits := []PlanCommit{{SHA: "aaa"}, {SHA: "bbb"}, {SHA: "ccc"}}

	got := remainingAfter(commits, "bbb")
	if len(got) != 1 || got[0].SHA != "ccc" {
		t.Errorf("expected [ccc], got %+v", got)
	}

	got = remainingAfter(commits, "ccc")
	if len(got) != 0 {
		t.Errorf("expected empty for last SHA, got %+v", got)
	}

	got = remainingAfter(commits, "missing")
	if len(got) != 0 {
		t.Errorf("expected empty for missing SHA, got %+v", got)
	}

	// Prefix match (short SHA from CHERRY_PICK_HEAD vs full SHA in plan)
	got = remainingAfter([]PlanCommit{{SHA: "abcdef1234"}, {SHA: "deadbeef99"}}, "abcdef")
	if len(got) != 1 || got[0].SHA != "deadbeef99" {
		t.Errorf("prefix match failed, got %+v", got)
	}
}

// runOnceCwd is a one-off git invocation that bypasses package vars
// (used by tests that want to set up state without disturbing the
// production runGit override semantics).
func runOnceCwd(cwd string, args ...string) error {
	out, err := runGitOutputCwdRaw(cwd, args...)
	_ = out
	return err
}

// runGitOutputCwdRaw spawns git directly (no Stderr inherit; tests
// don't want noise in passing runs).
func runGitOutputCwdRaw(cwd string, args ...string) (string, error) {
	cmd := newRawCmd(cwd, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
