package finalizecmd

import (
	"bytes"
	"strings"
	"testing"
)

// dummyConfirm returns the same answer every time. Used to suppress
// the dirty-WT prompt in tests that don't exercise it.
func dummyConfirm(answer bool) confirmFn {
	return func(prompt string, defaultYes bool) (bool, error) {
		return answer, nil
	}
}

func TestPreflight_notARepoErrors(t *testing.T) {
	// Run in a tempdir that is NOT a git repo.
	dir := t.TempDir()
	pinGitToCwd(t, dir)

	plan := &Plan{TargetBranch: "x"}
	var buf bytes.Buffer
	_, err := runPreflight(&buf, plan, dummyConfirm(true), "p:l")
	if err == nil || !strings.Contains(err.Error(), "not a git repository") {
		t.Fatalf("expected not-a-git-repo error, got: %v", err)
	}
}

func TestPreflight_localBranchCollision(t *testing.T) {
	repo := gitInTempRepo(t)
	commit(t, repo, "x.txt", "a", "init")
	mustGitCwd(t, repo, "branch", "ready")
	pinGitToCwd(t, repo)

	plan := &Plan{TargetBranch: "ready"}
	var buf bytes.Buffer
	_, err := runPreflight(&buf, plan, dummyConfirm(true), "p:l")
	if err == nil || !strings.Contains(err.Error(), "ready") {
		t.Fatalf("expected local branch collision error, got: %v", err)
	}
}

func TestPreflight_dirtyWT_stashYes(t *testing.T) {
	repo := gitInTempRepo(t)
	commit(t, repo, "x.txt", "a", "init")
	// Set up a fake origin so ls-remote works.
	originPath := t.TempDir()
	mustGitCwd(t, originPath, "init", "--bare")
	mustGitCwd(t, repo, "remote", "add", "origin", originPath)
	mustGitCwd(t, repo, "push", "origin", "main")
	// Dirty the WT.
	writeFile(t, repo, "x.txt", "modified")
	pinGitToCwd(t, repo)

	plan := &Plan{TargetBranch: "newbr"}
	var buf bytes.Buffer
	res, err := runPreflight(&buf, plan, dummyConfirm(true), "p:l")
	if err != nil {
		t.Fatalf("preflight: %v\n%s", err, buf.String())
	}
	if !res.Stashed {
		t.Errorf("expected Stashed=true")
	}
	if !strings.Contains(res.StashMessage, "jamsesh finalize-run p:l") {
		t.Errorf("unexpected stash message: %q", res.StashMessage)
	}

	// Verify stash exists.
	stashList := gitOutputCwd(t, repo, "stash", "list")
	if !strings.Contains(stashList, "jamsesh finalize-run p:l") {
		t.Errorf("stash not created: %q", stashList)
	}
}

func TestPreflight_dirtyWT_stashNoBails(t *testing.T) {
	repo := gitInTempRepo(t)
	commit(t, repo, "x.txt", "a", "init")
	writeFile(t, repo, "x.txt", "modified")
	pinGitToCwd(t, repo)

	plan := &Plan{TargetBranch: "newbr"}
	var buf bytes.Buffer
	_, err := runPreflight(&buf, plan, dummyConfirm(false), "p:l")
	if err == nil || !strings.Contains(err.Error(), "dirty") {
		t.Fatalf("expected dirty-WT bail when user said no, got: %v", err)
	}
}

func TestPreflight_originReachableHappy(t *testing.T) {
	repo := gitInTempRepo(t)
	commit(t, repo, "x.txt", "a", "init")
	originPath := t.TempDir()
	mustGitCwd(t, originPath, "init", "--bare")
	mustGitCwd(t, repo, "remote", "add", "origin", originPath)
	mustGitCwd(t, repo, "push", "origin", "main")
	pinGitToCwd(t, repo)

	plan := &Plan{TargetBranch: "newbr"}
	var buf bytes.Buffer
	res, err := runPreflight(&buf, plan, dummyConfirm(true), "p:l")
	if err != nil {
		t.Fatalf("preflight: %v\n%s", err, buf.String())
	}
	if res.Stashed {
		t.Errorf("did not expect stash on clean WT")
	}
	if res.OriginalBranch != "main" {
		t.Errorf("OriginalBranch = %q, want main", res.OriginalBranch)
	}
}

func TestPreflight_originUnreachableErrors(t *testing.T) {
	repo := gitInTempRepo(t)
	commit(t, repo, "x.txt", "a", "init")
	// add an origin pointing at a non-existent path
	mustGitCwd(t, repo, "remote", "add", "origin", "/nonexistent/path/12345.git")
	pinGitToCwd(t, repo)

	plan := &Plan{TargetBranch: "newbr"}
	var buf bytes.Buffer
	_, err := runPreflight(&buf, plan, dummyConfirm(true), "p:l")
	if err == nil || !strings.Contains(err.Error(), "origin remote unreachable") {
		t.Fatalf("expected origin-unreachable error, got: %v", err)
	}
}
