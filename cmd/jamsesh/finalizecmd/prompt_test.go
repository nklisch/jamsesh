package finalizecmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestConfirm(t *testing.T) {
	cases := []struct {
		name       string
		input      string
		defaultYes bool
		want       bool
		wantErr    bool
	}{
		{"empty defaults to yes", "\n", true, true, false},
		{"empty defaults to no", "\n", false, false, false},
		{"y", "y\n", false, true, false},
		{"Y uppercase", "Y\n", false, true, false},
		{"yes word", "yes\n", false, true, false},
		{"n", "n\n", true, false, false},
		{"N uppercase", "N\n", true, false, false},
		{"no word", "no\n", true, false, false},
		{"empty input no newline defaults", "", true, true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			old := stdin
			stdin = strings.NewReader(c.input)
			t.Cleanup(func() { stdin = old })

			var buf bytes.Buffer
			got, err := confirm(&buf, "test?", c.defaultYes)
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got: %v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != c.want {
				t.Errorf("got %v want %v", got, c.want)
			}
		})
	}
}

func TestConfirm_invalidThenValid(t *testing.T) {
	old := stdin
	stdin = strings.NewReader("maybe\nokay\nyes\n")
	t.Cleanup(func() { stdin = old })

	var buf bytes.Buffer
	got, err := confirm(&buf, "ok?", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got {
		t.Errorf("got false, want true after eventual yes")
	}
	if !strings.Contains(buf.String(), "Please answer y or n") {
		t.Errorf("prompt output missing re-prompt text: %q", buf.String())
	}
}

func TestConfirm_threeStrikeInvalid(t *testing.T) {
	old := stdin
	stdin = strings.NewReader("a\nb\nc\nd\n")
	t.Cleanup(func() { stdin = old })

	var buf bytes.Buffer
	_, err := confirm(&buf, "ok?", false)
	if err == nil {
		t.Fatalf("expected error after 3 invalid attempts, got nil")
	}
}
