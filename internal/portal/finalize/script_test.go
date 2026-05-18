package finalize_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"jamsesh/internal/portal/finalize"
)

// scriptInputCase describes one golden-test scenario for BuildScript.
type scriptInputCase struct {
	name           string
	mode           string
	target         string
	base           string
	shas           []string
	squashBody     string
	goldenFilename string
}

// fixedSquashBody is a sample composed message body used in the script
// goldens. It includes a body bullet list and a co-author trailer so the
// heredoc shape is exercised end-to-end.
const fixedSquashBody = `Ship the comments feature

- feat: comments REST endpoint
- feat: comments WebSocket gateway
- docs: document comments protocol

Co-authored-by: Alice Smith <alice@example.com>
Co-authored-by: Bob Tan <bob@example.com>
`

func TestBuildScript_Goldens(t *testing.T) {
	one := []string{
		"1111111111111111111111111111111111111111",
	}
	three := []string{
		"1111111111111111111111111111111111111111",
		"2222222222222222222222222222222222222222",
		"3333333333333333333333333333333333333333",
	}
	ten := []string{
		"0000000000000000000000000000000000000001",
		"0000000000000000000000000000000000000002",
		"0000000000000000000000000000000000000003",
		"0000000000000000000000000000000000000004",
		"0000000000000000000000000000000000000005",
		"0000000000000000000000000000000000000006",
		"0000000000000000000000000000000000000007",
		"0000000000000000000000000000000000000008",
		"0000000000000000000000000000000000000009",
		"000000000000000000000000000000000000000a",
	}

	const target = "ship/comments"
	const base = "fedcba9876543210fedcba9876543210fedcba98"

	cases := []scriptInputCase{
		{
			name:           "squash_1_commit",
			mode:           "squash",
			target:         target,
			base:           base,
			shas:           one,
			squashBody:     fixedSquashBody,
			goldenFilename: "squash_script_1commit.golden.txt",
		},
		{
			name:           "squash_3_commits",
			mode:           "squash",
			target:         target,
			base:           base,
			shas:           three,
			squashBody:     fixedSquashBody,
			goldenFilename: "squash_script_3commits.golden.txt",
		},
		{
			name:           "squash_10_commits",
			mode:           "squash",
			target:         target,
			base:           base,
			shas:           ten,
			squashBody:     fixedSquashBody,
			goldenFilename: "squash_script_10commits.golden.txt",
		},
		{
			name:           "preserve_1_commit",
			mode:           "preserve",
			target:         target,
			base:           base,
			shas:           one,
			goldenFilename: "preserve_script_1commit.golden.txt",
		},
		{
			name:           "preserve_3_commits",
			mode:           "preserve",
			target:         target,
			base:           base,
			shas:           three,
			goldenFilename: "preserve_script_3commits.golden.txt",
		},
		{
			name:           "preserve_10_commits",
			mode:           "preserve",
			target:         target,
			base:           base,
			shas:           ten,
			goldenFilename: "preserve_script_10commits.golden.txt",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := finalize.BuildScript(finalize.ScriptInput{
				Mode:              tc.mode,
				TargetBranch:      tc.target,
				BaseSHA:           tc.base,
				SelectedSHAs:      tc.shas,
				SquashMessageBody: tc.squashBody,
			})
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

func TestBuildScript_SquashCarriesPlaceholdersAndSafeHeader(t *testing.T) {
	got := finalize.BuildScript(finalize.ScriptInput{
		Mode:              "squash",
		TargetBranch:      "ship/foo",
		BaseSHA:           "deadbeefcafebabe",
		SelectedSHAs:      []string{"abc123"},
		SquashMessageBody: "Subject\n",
	})
	mustContain := []string{
		"#!/usr/bin/env bash",
		"set -euo pipefail",
		"$JAMSESH_FETCH_REMOTE",
		"$JAMSESH_RUNNER_NAME",
		"$JAMSESH_RUNNER_EMAIL",
		"<<'JAMSESH_MSG'",
		"JAMSESH_MSG\n",
		"==> Composing squash commit",
	}
	for _, m := range mustContain {
		if !strings.Contains(got, m) {
			t.Errorf("squash script missing %q\n--- script ---\n%s", m, got)
		}
	}
}

func TestBuildScript_PreserveOnePickPerSHA(t *testing.T) {
	shas := []string{"aaa", "bbb", "ccc"}
	got := finalize.BuildScript(finalize.ScriptInput{
		Mode:         "preserve",
		TargetBranch: "ship/x",
		BaseSHA:      "deadbeef",
		SelectedSHAs: shas,
	})
	for _, sha := range shas {
		if !strings.Contains(got, "git cherry-pick "+sha) {
			t.Errorf("preserve script missing git cherry-pick %s", sha)
		}
	}
	// Should NOT contain --no-commit (that's the squash variant).
	if strings.Contains(got, "--no-commit") {
		t.Error("preserve script should not contain --no-commit")
	}
	// Should NOT have a heredoc / JAMSESH_MSG sentinel.
	if strings.Contains(got, "JAMSESH_MSG") {
		t.Error("preserve script should not contain JAMSESH_MSG heredoc")
	}
}

func TestBuildScript_Deterministic(t *testing.T) {
	in := finalize.ScriptInput{
		Mode:              "squash",
		TargetBranch:      "ship/det",
		BaseSHA:           "deadbeef",
		SelectedSHAs:      []string{"a", "b", "c"},
		SquashMessageBody: "Subject\n",
	}
	a := finalize.BuildScript(in)
	b := finalize.BuildScript(in)
	if a != b {
		t.Error("BuildScript not deterministic across two invocations")
	}
}

