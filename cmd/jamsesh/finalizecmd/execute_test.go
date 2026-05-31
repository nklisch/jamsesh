package finalizecmd

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// setupFeatureBranch creates the standard test repo layout:
//
//	main:     b -> base (a.txt = "base")
//	feature:  base -> f1 (b.txt = "f1") -> f2 (b.txt = "f2 + line")
//
// Returns (repoPath, baseSHA, f1SHA, f2SHA).
func setupFeatureBranch(t *testing.T) (string, string, string, string) {
	t.Helper()
	repo := gitInTempRepo(t)
	baseSHA := commit(t, repo, "a.txt", "base\n", "base commit")
	mustGitCwd(t, repo, "checkout", "-b", "feature")
	f1SHA := commit(t, repo, "b.txt", "f1\n", "feat: first")
	f2SHA := commit(t, repo, "c.txt", "f2\n", "feat: second")
	mustGitCwd(t, repo, "checkout", "main")
	return repo, baseSHA, f1SHA, f2SHA
}

func TestExecute_preserveHappy(t *testing.T) {
	repo, baseSHA, f1SHA, f2SHA := setupFeatureBranch(t)
	pinGitToCwd(t, repo)

	plan := &Plan{
		PlanID:       "sess1:lock1",
		Mode:         "preserve",
		TargetBranch: "ready",
		BaseSHA:      baseSHA,
		SelectedCommits: []PlanCommit{
			{SHA: f1SHA, Subject: "feat: first"},
			{SHA: f2SHA, Subject: "feat: second"},
		},
	}

	var buf bytes.Buffer
	err := execute(&buf, plan, runnerIdentity{Name: "Runner", Email: "r@example.com"})
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, buf.String())
	}

	// Expect 3 commits on ready (base + 2 picks).
	log := gitOutputCwd(t, repo, "log", "--format=%s", "ready")
	wantLines := []string{"feat: second", "feat: first", "base commit"}
	gotLines := strings.Split(strings.TrimSpace(log), "\n")
	if len(gotLines) != len(wantLines) {
		t.Fatalf("commit count mismatch: got %d (%v), want %d", len(gotLines), gotLines, len(wantLines))
	}
	for i, w := range wantLines {
		if gotLines[i] != w {
			t.Errorf("commit %d subject = %q, want %q", i, gotLines[i], w)
		}
	}

	// Verbose logging should include +git lines.
	if !strings.Contains(buf.String(), "+ git checkout -b ready") {
		t.Errorf("missing verbose checkout log:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "+ git cherry-pick") {
		t.Errorf("missing verbose cherry-pick log:\n%s", buf.String())
	}
}

func TestExecute_squashHappy(t *testing.T) {
	repo, baseSHA, f1SHA, f2SHA := setupFeatureBranch(t)
	pinGitToCwd(t, repo)

	msg := "Add feature\n\nCo-authored-by: Alice <a@example.com>\nCo-authored-by: Bob <b@example.com>\n"
	plan := &Plan{
		PlanID:       "sess1:lock1",
		Mode:         "squash",
		TargetBranch: "ready",
		BaseSHA:      baseSHA,
		SelectedCommits: []PlanCommit{
			{SHA: f1SHA, Subject: "feat: first"},
			{SHA: f2SHA, Subject: "feat: second"},
		},
		CommitMessage: msg,
	}

	var buf bytes.Buffer
	err := execute(&buf, plan, runnerIdentity{Name: "Runner Person", Email: "runner@example.com"})
	if err != nil {
		t.Fatalf("execute: %v\n%s", err, buf.String())
	}

	// One new commit on ready beyond base.
	log := gitOutputCwd(t, repo, "log", "--format=%s", "ready")
	lines := strings.Split(strings.TrimSpace(log), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 commits (squash + base), got %d: %v", len(lines), lines)
	}
	if lines[0] != "Add feature" {
		t.Errorf("squash commit subject = %q, want %q", lines[0], "Add feature")
	}

	body := gitOutputCwd(t, repo, "log", "-1", "--format=%B", "ready")
	if !strings.Contains(body, "Co-authored-by: Alice <a@example.com>") {
		t.Errorf("missing Alice trailer:\n%s", body)
	}
	if !strings.Contains(body, "Co-authored-by: Bob <b@example.com>") {
		t.Errorf("missing Bob trailer:\n%s", body)
	}

	// Authorship is the runner, NOT one of the original authors.
	author := gitOutputCwd(t, repo, "log", "-1", "--format=%an <%ae>", "ready")
	if author != "Runner Person <runner@example.com>" {
		t.Errorf("squash commit author = %q, want runner identity", author)
	}
}

func TestExecute_conflictReturnsConflictError(t *testing.T) {
	// Set up a repo where cherry-pick will conflict. base has x.txt="base";
	// feature changes it to "feature change"; main changes it to "main change".
	// Branching `ready` from main and cherry-picking feature's commit triggers
	// a 3-way merge conflict because both ancestors of the merge differ from
	// the base in incompatible ways.
	repo := gitInTempRepo(t)
	commit(t, repo, "x.txt", "base\n", "base commit")
	mustGitCwd(t, repo, "checkout", "-b", "feature")
	conflictSHA := commit(t, repo, "x.txt", "feature change\n", "feat: change")
	mustGitCwd(t, repo, "checkout", "main")
	// Modify the same line on main — diverged from base in an incompatible way.
	mainHEAD := commit(t, repo, "x.txt", "main change\n", "main: change")

	pinGitToCwd(t, repo)

	plan := &Plan{
		PlanID:       "sess1:lock1",
		Mode:         "preserve",
		TargetBranch: "ready",
		// Branch ready from main's tip so the cherry-pick of feature's
		// commit produces a true 3-way conflict.
		BaseSHA: mainHEAD,
		SelectedCommits: []PlanCommit{
			{SHA: conflictSHA, Subject: "feat: change"},
		},
	}

	var buf bytes.Buffer
	err := execute(&buf, plan, runnerIdentity{Name: "R", Email: "r@e.com"})
	if err == nil {
		t.Fatalf("expected conflict error, got nil\n%s", buf.String())
	}
	var conf *conflictError
	if !errors.As(err, &conf) {
		t.Fatalf("expected *conflictError, got %T: %v", err, err)
	}
	if conf.OffendingSHA == "" {
		t.Errorf("conflictError.OffendingSHA empty")
	}

	// CHERRY_PICK_HEAD should still exist post-call so the user can resume.
	sha := detectMidPickAt(t, repo)
	if sha == "" {
		t.Errorf("CHERRY_PICK_HEAD missing post-conflict")
	}
}

func TestRenderConflict(t *testing.T) {
	e := &conflictError{
		OffendingSHA: "abc123",
		Remaining: []PlanCommit{
			{SHA: "def456", Subject: "next"},
		},
	}
	var buf bytes.Buffer
	renderConflict(&buf, e)
	out := buf.String()
	for _, want := range []string{
		"Cherry-pick failed at abc123",
		"def456  next",
		"git cherry-pick --continue",
		"git cherry-pick --abort",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestExecute_unknownModeErrors(t *testing.T) {
	repo, baseSHA, f1SHA, _ := setupFeatureBranch(t)
	pinGitToCwd(t, repo)
	plan := &Plan{
		Mode:            "rebase-merge",
		TargetBranch:    "x",
		BaseSHA:         baseSHA,
		SelectedCommits: []PlanCommit{{SHA: f1SHA}},
	}
	var buf bytes.Buffer
	err := execute(&buf, plan, runnerIdentity{Name: "R", Email: "r@e.com"})
	if err == nil {
		t.Fatalf("expected error for unknown mode")
	}
	if !strings.Contains(err.Error(), "rebase-merge") {
		t.Errorf("error did not echo mode: %v", err)
	}
}

func TestRedactGitArgs(t *testing.T) {
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "bearer token redacted",
			in:   []string{"-c", "http.extraHeader=Authorization: Bearer abc123"},
			want: []string{"-c", "http.extraHeader=Authorization: Bearer <redacted>"},
		},
		{
			name: "basic token redacted",
			in:   []string{"-c", "http.extraHeader=Authorization: Basic xyz789"},
			want: []string{"-c", "http.extraHeader=Authorization: Basic <redacted>"},
		},
		{
			name: "non-Authorization -c arg unchanged",
			in:   []string{"-c", "color.ui=always"},
			want: []string{"-c", "color.ui=always"},
		},
		{
			name: "mixed args only redacts Authorization",
			in:   []string{"-c", "http.extraHeader=Authorization: Bearer tok", "fetch", "jamsesh"},
			want: []string{"-c", "http.extraHeader=Authorization: Bearer <redacted>", "fetch", "jamsesh"},
		},
		{
			name: "no extraHeader args pass through untouched",
			in:   []string{"cherry-pick", "--no-commit", "deadbeef"},
			want: []string{"cherry-pick", "--no-commit", "deadbeef"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := redactGitArgs(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d; got %v", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("arg[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

func TestRunGitVerbose_redactsAuthorizationInPrint(t *testing.T) {
	// runGitVerbose will actually try to run git, so we need a real repo.
	repo := gitInTempRepo(t)
	pinGitToCwd(t, repo)

	var buf bytes.Buffer
	// Use the legacy argv shape as a regression fixture for redaction. Git
	// ignores the header at runtime but runGitVerbose still prints the arg.
	_ = runGitVerbose(&buf, "-c", "http.extraHeader=Authorization: Bearer supersecret", "status")

	printed := buf.String()
	if strings.Contains(printed, "supersecret") {
		t.Errorf("raw token leaked in printed line:\n%s", printed)
	}
	if !strings.Contains(printed, "<redacted>") {
		t.Errorf("<redacted> not present in printed line:\n%s", printed)
	}
	if !strings.Contains(printed, "Authorization: Bearer") {
		t.Errorf("auth scheme stripped unexpectedly from printed line:\n%s", printed)
	}
}

func TestExecute_emptyPlanErrors(t *testing.T) {
	repo := gitInTempRepo(t)
	pinGitToCwd(t, repo)
	plan := &Plan{Mode: "preserve", TargetBranch: "x", BaseSHA: "HEAD"}
	var buf bytes.Buffer
	err := execute(&buf, plan, runnerIdentity{Name: "R", Email: "r@e.com"})
	if err == nil || !strings.Contains(err.Error(), "no commits") {
		t.Errorf("expected 'no commits' error, got %v", err)
	}
}

func TestResolveRunnerIdentity(t *testing.T) {
	repo := gitInTempRepo(t)
	pinGitToCwd(t, repo)
	id, err := resolveRunnerIdentity()
	if err != nil {
		t.Fatalf("resolveRunnerIdentity: %v", err)
	}
	if id.Name != "Test User" || id.Email != "test@example.com" {
		t.Errorf("unexpected identity: %+v", id)
	}
}
