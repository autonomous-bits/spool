package main

import (
	"io"
	"log/slog"

	"github.com/spf13/cobra"
)

func bootstrapRootCommand(stdout io.Writer) (*cobra.Command, func() error) {
	return newRootCommand(stdout), func() error { return nil }
}

func newLogger(output io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{})).With("component", "spl")
}
