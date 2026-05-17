package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/urfave/cli/v3"

	"jamsesh/cmd/jamsesh/auth"
	"jamsesh/cmd/jamsesh/mcpheaders"
	"jamsesh/internal/buildinfo"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	app := &cli.Command{
		Name:    "jamsesh",
		Usage:   "Local client for the jamsesh portal",
		Version: buildinfo.Version,
		Commands: []*cli.Command{
			auth.Command(),
			mcpheaders.Command(),
			// session-commands and hooks land in sibling features
		},
	}

	if err := app.Run(ctx, os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
