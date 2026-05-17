package finalizecmd

import (
	"bytes"
	"strings"
	"testing"
)

func samplePreservePlan() *Plan {
	return &Plan{
		PlanID:       "sess1:lock1",
		Mode:         "preserve",
		TargetBranch: "feature/ready",
		BaseSHA:      "abc1234",
		SelectedCommits: []PlanCommit{
			{SHA: "deadbeef0001", Subject: "first commit", AuthorName: "Alice"},
			{SHA: "deadbeef0002", Subject: "second commit", AuthorName: "Bob"},
		},
	}
}

func sampleSquashPlan() *Plan {
	return &Plan{
		PlanID:       "sess1:lock1",
		Mode:         "squash",
		TargetBranch: "feature/squash",
		BaseSHA:      "abc1234",
		SelectedCommits: []PlanCommit{
			{SHA: "deadbeef0001", Subject: "first", AuthorName: "Alice"},
			{SHA: "deadbeef0002", Subject: "second", AuthorName: "Bob"},
		},
		CommitMessage: "Add the feature\n\nCo-authored-by: Alice <a@example.com>\nCo-authored-by: Bob <b@example.com>\n",
		CoAuthors: []CoAuthor{
			{Name: "Alice", Email: "a@example.com"},
			{Name: "Bob", Email: "b@example.com"},
		},
	}
}

func TestPrintScript_preserve(t *testing.T) {
	var buf bytes.Buffer
	if err := printScript(&buf, samplePreservePlan()); err != nil {
		t.Fatalf("printScript: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "git checkout -b feature/ready abc1234") {
		t.Errorf("missing checkout step: %s", out)
	}
	if !strings.Contains(out, "git cherry-pick deadbeef0001") {
		t.Errorf("missing per-commit cherry-pick: %s", out)
	}
	if !strings.Contains(out, "git cherry-pick deadbeef0002") {
		t.Errorf("missing second cherry-pick: %s", out)
	}
}

func TestPrintScript_squash(t *testing.T) {
	var buf bytes.Buffer
	if err := printScript(&buf, sampleSquashPlan()); err != nil {
		t.Fatalf("printScript: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "git cherry-pick --no-commit deadbeef0001 deadbeef0002") {
		t.Errorf("missing --no-commit cherry-pick: %s", out)
	}
	if !strings.Contains(out, "JAMSESH_EOF") {
		t.Errorf("missing heredoc terminator: %s", out)
	}
	if !strings.Contains(out, "Co-authored-by: Alice") {
		t.Errorf("missing composed message body: %s", out)
	}
}

func TestPrintScript_unknownMode(t *testing.T) {
	bad := &Plan{Mode: "rebase", TargetBranch: "x", BaseSHA: "y"}
	var buf bytes.Buffer
	if err := printScript(&buf, bad); err == nil {
		t.Errorf("expected error for unknown mode, got nil")
	}
}

func TestPickHeredocTerminator_unique(t *testing.T) {
	body := "JAMSESH_EOF\nstill in message"
	term, err := pickHeredocTerminator(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if term == "JAMSESH_EOF" {
		t.Errorf("terminator collided with body content")
	}
	if containsStandaloneLine(body, term) {
		t.Errorf("picked terminator %q still appears in body", term)
	}
}

func TestPrintPlanSummary(t *testing.T) {
	var buf bytes.Buffer
	printPlanSummary(&buf, sampleSquashPlan())
	out := buf.String()
	for _, want := range []string{
		"Mode:          squash",
		"Target branch: feature/squash",
		"deadbeef0001",
		"Co-authors:",
		"Alice <a@example.com>",
		"Add the feature",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q\n--- got ---\n%s", want, out)
		}
	}
}
