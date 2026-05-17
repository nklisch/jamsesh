// Package hookio provides a generic JSON-in / JSON-out scaffold for Claude
// Code hook subcommands. Each hook subcommand type-parameterizes Run with
// its own input and output structs; hookio handles stdin decode, handler
// dispatch, and stdout encode consistently.
package hookio

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
)

// errorEnvelope is the error shape written to out when the handler returns
// an error. It mirrors the portal's error envelope for cross-surface
// consistency.
type errorEnvelope struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// Run reads JSON from in, decodes into a value of type I, calls handle, and
// encodes the resulting value of type O to out. If decoding or handling
// fails, an errorEnvelope is written to out and the error is returned to
// the caller so the subcommand can set an appropriate exit code.
func Run[I, O any](ctx context.Context, in io.Reader, out io.Writer, handle func(context.Context, I) (O, error)) error {
	raw, err := io.ReadAll(in)
	if err != nil {
		writeError(out, "read_error", fmt.Sprintf("reading stdin: %v", err))
		return fmt.Errorf("hookio: reading stdin: %w", err)
	}

	var input I
	if err := json.Unmarshal(raw, &input); err != nil {
		writeError(out, "decode_error", fmt.Sprintf("decoding JSON input: %v", err))
		return fmt.Errorf("hookio: decoding JSON input: %w", err)
	}

	output, err := handle(ctx, input)
	if err != nil {
		writeError(out, "handler_error", err.Error())
		return fmt.Errorf("hookio: handler error: %w", err)
	}

	if err := json.NewEncoder(out).Encode(output); err != nil {
		return fmt.Errorf("hookio: encoding JSON output: %w", err)
	}
	return nil
}

func writeError(out io.Writer, code, message string) {
	env := errorEnvelope{Error: code, Message: message}
	// Best-effort; ignore encoding/write errors since we're already in an
	// error path.
	_ = json.NewEncoder(out).Encode(env)
}
