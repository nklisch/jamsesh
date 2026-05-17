package hooks

import (
	"context"
	"io"
	"os"
)

type ctxKey int

const (
	stdinKey  ctxKey = iota
	stdoutKey ctxKey = iota
)

// WithIO injects custom stdin/stdout into a context for testing. Production
// code uses the real os.Stdin / os.Stdout via stdinOf / stdoutOf.
func WithIO(ctx context.Context, in io.Reader, out io.Writer) context.Context {
	ctx = context.WithValue(ctx, stdinKey, in)
	ctx = context.WithValue(ctx, stdoutKey, out)
	return ctx
}

func stdinOf(ctx context.Context) io.Reader {
	if v, ok := ctx.Value(stdinKey).(io.Reader); ok {
		return v
	}
	return os.Stdin
}

func stdoutOf(ctx context.Context) io.Writer {
	if v, ok := ctx.Value(stdoutKey).(io.Writer); ok {
		return v
	}
	return os.Stdout
}
