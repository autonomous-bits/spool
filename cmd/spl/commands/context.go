package commands

import (
	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/resolve"
	"github.com/spf13/cobra"
)

// NewContextCommand creates the spl context command.
func NewContextCommand(toolProvider func() (*resolve.ResolveTool, error), repoProvider func() (*repository.Repository, error)) *cobra.Command {
	command := newContextualCommand(
		"context",
		"Assemble evidence-focused graph context",
		"Return JSON evidence and bounded graph context from a lexical query or typed filters.",
		"spl context --branch main --label Task --property-text status=open --direction both",
		toolProvider,
		true,
	)
	command.AddCommand(NewContextInitCommand())
	command.AddCommand(NewContextExportCommand(repoProvider))
	return command
}
