package hookio

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type echoInput struct {
	Message string `json:"message"`
}

type echoOutput struct {
	Echo string `json:"echo"`
}

func echoHandler(_ context.Context, in echoInput) (echoOutput, error) {
	return echoOutput{Echo: in.Message}, nil
}

// TestRun_happyPath verifies that a valid JSON input round-trips through the
// handler and is written as JSON to the output writer.
func TestRun_happyPath(t *testing.T) {
	in := strings.NewReader(`{"message":"hello"}`)
	var out bytes.Buffer

	if err := Run(context.Background(), in, &out, echoHandler); err != nil {
		t.Fatalf("Run: unexpected error: %v", err)
	}

	var got echoOutput
	if err := json.NewDecoder(&out).Decode(&got); err != nil {
		t.Fatalf("decoding output: %v", err)
	}
	if got.Echo != "hello" {
		t.Errorf("echo = %q, want %q", got.Echo, "hello")
	}
}

// TestRun_malformedJSON verifies that malformed input causes Run to return
// an error and write an error envelope to out.
func TestRun_malformedJSON(t *testing.T) {
	in := strings.NewReader(`not valid json`)
	var out bytes.Buffer

	err := Run(context.Background(), in, &out, echoHandler)
	if err == nil {
		t.Fatal("Run with malformed JSON: expected error, got nil")
	}

	var env errorEnvelope
	if decErr := json.NewDecoder(&out).Decode(&env); decErr != nil {
		t.Fatalf("decoding error envelope: %v", decErr)
	}
	if env.Error == "" {
		t.Error("error envelope has empty Error field")
	}
	if env.Message == "" {
		t.Error("error envelope has empty Message field")
	}
}

// TestRun_handlerError verifies that a handler-returned error causes Run to
// write an error envelope and return the error.
func TestRun_handlerError(t *testing.T) {
	in := strings.NewReader(`{"message":"trigger-error"}`)
	var out bytes.Buffer

	failHandler := func(_ context.Context, in echoInput) (echoOutput, error) {
		return echoOutput{}, errors.New("intentional handler failure")
	}

	err := Run(context.Background(), in, &out, failHandler)
	if err == nil {
		t.Fatal("Run with failing handler: expected error, got nil")
	}

	var env errorEnvelope
	if decErr := json.NewDecoder(&out).Decode(&env); decErr != nil {
		t.Fatalf("decoding error envelope: %v", decErr)
	}
	if !strings.Contains(env.Message, "intentional handler failure") {
		t.Errorf("error message %q does not contain expected text", env.Message)
	}
}

// TestRun_emptyInput verifies that an empty JSON object `{}` is accepted and
// results in a zero-value input being passed to the handler.
func TestRun_emptyInput(t *testing.T) {
	in := strings.NewReader(`{}`)
	var out bytes.Buffer

	if err := Run(context.Background(), in, &out, echoHandler); err != nil {
		t.Fatalf("Run with empty object: unexpected error: %v", err)
	}

	var got echoOutput
	if err := json.NewDecoder(&out).Decode(&got); err != nil {
		t.Fatalf("decoding output: %v", err)
	}
	// Zero value for string is ""; echo should pass it through.
	if got.Echo != "" {
		t.Errorf("echo = %q, want empty string", got.Echo)
	}
}
