package finalize_test

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-git/go-git/v5/plumbing/object"

	"jamsesh/internal/portal/finalize"
)

// -update overwrites the golden files with the current output. Run as
//
//	go test ./internal/portal/finalize -run TestComposeSquashMessage -update
//
// to refresh them when intentionally changing the composer output.
var updateGoldens = flag.Bool("update", false, "rewrite golden files in testdata/")

// fakeCommit fabricates an *object.Commit with the given fields. Hashes and
// committer-when default to deterministic stubs; the composer never reads
// hash/committer.
func fakeCommit(authorName, authorEmail, message string) *object.Commit {
	return &object.Commit{
		Author: object.Signature{
			Name:  authorName,
			Email: authorEmail,
			When:  time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC),
		},
		Committer: object.Signature{
			Name:  authorName,
			Email: authorEmail,
			When:  time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC),
		},
		Message: message,
	}
}

func TestComposeSquashMessage_Golden(t *testing.T) {
	cases := []struct {
		name           string
		sessionGoal    string
		override       string
		commits        []*object.Commit
		goldenFilename string
	}{
		{
			name:        "single_author_three_commits",
			sessionGoal: "Wire up the auth flow",
			commits: []*object.Commit{
				fakeCommit("Alice Smith", "alice@example.com", "feat: add login form\n"),
				fakeCommit("Alice Smith", "alice@example.com", "feat: wire token storage\n"),
				fakeCommit("Alice Smith", "alice@example.com", "fix: clear stale session on logout\n"),
			},
			goldenFilename: "squash_message_single_author.golden.txt",
		},
		{
			name:        "three_distinct_authors_three_commits",
			sessionGoal: "Ship the comments feature",
			commits: []*object.Commit{
				fakeCommit("Alice Smith", "alice@example.com", "feat: comments REST endpoint\n"),
				fakeCommit("Bob Tan", "bob@example.com", "feat: comments WebSocket gateway\n"),
				fakeCommit("Carol Vasquez", "carol@example.com", "docs: document comments protocol\n"),
			},
			goldenFilename: "squash_message_three_authors.golden.txt",
		},
		{
			name:        "case_variant_emails_dedup",
			sessionGoal: "Refactor logging",
			commits: []*object.Commit{
				fakeCommit("Alice Smith", "Alice@example.com", "refactor: extract logger interface\n"),
				fakeCommit("Bob Tan", "bob@example.com", "refactor: route logs through interface\n"),
				fakeCommit("Alice Lower", "alice@example.com", "refactor: clean up callers\n"),
			},
			goldenFilename: "squash_message_case_variant.golden.txt",
		},
		{
			name:        "user_override_subject_multiline",
			sessionGoal: "Should be ignored",
			override:    "Manual squash subject line\nNot this second line\nNor this one\n",
			commits: []*object.Commit{
				fakeCommit("Alice Smith", "alice@example.com", "wip: experiment 1\n"),
				fakeCommit("Alice Smith", "alice@example.com", "wip: experiment 2\n"),
			},
			goldenFilename: "squash_message_user_override.golden.txt",
		},
		{
			name:        "session_goal_word_boundary_truncation",
			sessionGoal: "Wire up the authentication flow with magic-link senders and complete the password-reset journey end to end",
			commits: []*object.Commit{
				fakeCommit("Alice Smith", "alice@example.com", "feat: magic-link sender\n"),
			},
			goldenFilename: "squash_message_truncation.golden.txt",
		},
		{
			name:           "empty_selection",
			sessionGoal:    "Empty selection",
			commits:        nil,
			goldenFilename: "squash_message_empty.golden.txt",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			subject, body, cas := finalize.ComposeSquashMessage(tc.sessionGoal, tc.override, tc.commits)
			got := finalize.RenderSquashMessageBody(subject, body, cas)
			goldenPath := filepath.Join("testdata", tc.goldenFilename)
			if *updateGoldens {
				if err := os.WriteFile(goldenPath, []byte(got), 0o644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
			}
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("read golden %s: %v", goldenPath, err)
			}
			if got != string(want) {
				t.Errorf("golden mismatch %s\n--- got ---\n%s\n--- want ---\n%s",
					tc.goldenFilename, got, string(want))
			}
		})
	}
}

func TestComposeSquashMessage_CoAuthorOrderAndDedup(t *testing.T) {
	commits := []*object.Commit{
		fakeCommit("Alice", "Alice@example.com", "feat: a\n"),
		fakeCommit("Bob", "bob@example.com", "feat: b\n"),
		fakeCommit("Alice", "alice@example.com", "feat: c\n"), // duplicate by lowercase email
		fakeCommit("Carol", "carol@example.com", "feat: d\n"),
	}
	_, _, cas := finalize.ComposeSquashMessage("Goal", "", commits)
	if len(cas) != 3 {
		t.Fatalf("co-author count: got %d, want 3", len(cas))
	}
	wantOrder := []string{"Alice@example.com", "bob@example.com", "carol@example.com"}
	for i, w := range wantOrder {
		if cas[i].Email != w {
			t.Errorf("co-author %d email: got %q, want %q", i, cas[i].Email, w)
		}
	}
	// First-seen casing for the deduped author.
	if cas[0].Email != "Alice@example.com" {
		t.Errorf("dedup should preserve first-seen casing: got %q", cas[0].Email)
	}
}

func TestComposeSquashMessage_UserOverride_FirstLineOnly(t *testing.T) {
	commits := []*object.Commit{
		fakeCommit("Alice", "alice@example.com", "feat: a\n"),
	}
	subject, _, _ := finalize.ComposeSquashMessage("ignored", "Subject one\nSubject two\n", commits)
	if subject != "Subject one" {
		t.Errorf("subject: got %q, want %q", subject, "Subject one")
	}
}

func TestComposeSquashMessage_WordBoundaryTruncation(t *testing.T) {
	long := "Wire up the authentication flow with magic-link senders and complete the password-reset journey"
	subject, _, _ := finalize.ComposeSquashMessage(long, "", nil)
	if !strings.HasSuffix(subject, "…") {
		t.Errorf("expected truncated subject to end with ellipsis, got %q", subject)
	}
	// Length budget: 72 runes for the cut content + 1 rune for the ellipsis.
	// We never break in the middle of a word.
	if strings.Contains(subject, "complete the password-reset") {
		t.Errorf("subject should have been cut before that point, got %q", subject)
	}
}

func TestComposeSquashMessage_NoTruncation(t *testing.T) {
	short := "Short goal"
	subject, _, _ := finalize.ComposeSquashMessage(short, "", nil)
	if subject != short {
		t.Errorf("short goal should pass through: got %q, want %q", subject, short)
	}
}

func TestComposeSquashMessage_Empty_NoBodyNoCoAuthors(t *testing.T) {
	subject, body, cas := finalize.ComposeSquashMessage("Hello", "", nil)
	if subject != "Hello" {
		t.Errorf("subject: got %q, want %q", subject, "Hello")
	}
	if body != "" {
		t.Errorf("body should be empty, got %q", body)
	}
	if len(cas) != 0 {
		t.Errorf("co-authors should be empty, got %v", cas)
	}
}
