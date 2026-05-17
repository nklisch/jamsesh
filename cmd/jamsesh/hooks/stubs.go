package hooks

// stubs.go contains placeholder implementations for hook subcommands that are
// not yet implemented. Each stub reads the CC JSON payload from stdin, ignores
// it, and writes an empty JSON object to stdout. They will be replaced by
// real implementations in the sibling story
// (epic-cc-plugin-hooks-fetch-push-and-stop-hooks).

import (
	"context"
	"fmt"

	"github.com/urfave/cli/v3"

	"jamsesh/cmd/jamsesh/hookio"
)

// stubInput accepts any CC hook input without failing on unknown fields.
type stubInput struct{}

// stubOutput produces an empty JSON object, which is a valid no-op response
// for all CC hook types.
type stubOutput struct{}

func stubHandler(_ context.Context, _ stubInput) (stubOutput, error) {
	return stubOutput{}, nil
}

// SessionStart is the stub action for "jamsesh hook session-start".
// Replaced by a real implementation in fetch-push-and-stop-hooks.
func SessionStart(ctx context.Context, _ *cli.Command) error {
	if err := hookio.Run(ctx, stdinOf(ctx), stdoutOf(ctx), stubHandler); err != nil {
		return fmt.Errorf("session-start: %w", err)
	}
	return nil
}

// UserPromptSubmit is the stub action for "jamsesh hook user-prompt-submit".
// Replaced by a real implementation in fetch-push-and-stop-hooks.
func UserPromptSubmit(ctx context.Context, _ *cli.Command) error {
	if err := hookio.Run(ctx, stdinOf(ctx), stdoutOf(ctx), stubHandler); err != nil {
		return fmt.Errorf("user-prompt-submit: %w", err)
	}
	return nil
}

// PostToolUse is the stub action for "jamsesh hook post-tool-use".
// Replaced by a real implementation in fetch-push-and-stop-hooks.
func PostToolUse(ctx context.Context, _ *cli.Command) error {
	if err := hookio.Run(ctx, stdinOf(ctx), stdoutOf(ctx), stubHandler); err != nil {
		return fmt.Errorf("post-tool-use: %w", err)
	}
	return nil
}

// Stop is the stub action for "jamsesh hook stop".
// Replaced by a real implementation in fetch-push-and-stop-hooks.
func Stop(ctx context.Context, _ *cli.Command) error {
	if err := hookio.Run(ctx, stdinOf(ctx), stdoutOf(ctx), stubHandler); err != nil {
		return fmt.Errorf("stop: %w", err)
	}
	return nil
}
