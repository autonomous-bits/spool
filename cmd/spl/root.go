package main

import (
	"io"

	"github.com/autonomous-bits/spool/cmd/spl/commands"
	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/resolve"
	"github.com/spf13/cobra"
)

func newRootCommand(stdout io.Writer, tool *resolve.ResolveTool) *cobra.Command {
	return newRootCommandWithLifecycle(stdout, func() (*resolve.ResolveTool, error) {
		return tool, nil
	}, func() (*repository.Repository, error) {
		return repository.NewSeedRepository(), nil
	})
}

func newRootCommandWithLifecycle(
	stdout io.Writer,
	toolProvider func() (*resolve.ResolveTool, error),
	initialize func() (*repository.Repository, error),
) *cobra.Command {
	root := &cobra.Command{
		Use:          "spl",
		SilenceUsage: true,
	}
	root.SetOut(stdout)
	root.AddCommand(commands.NewInitCommand(initialize))
	root.AddCommand(commands.NewAddCommand(toolProvider))
	root.AddCommand(commands.NewStatusCommand(toolProvider))
	root.AddCommand(commands.NewCommitCommand(toolProvider))
	root.AddCommand(commands.NewBranchCommandWithToolProvider(toolProvider))
	root.AddCommand(commands.NewSwitchCommand(toolProvider))
	root.AddCommand(commands.NewResolveCommandWithToolProvider(toolProvider))
	root.AddCommand(commands.NewDiffCommand(toolProvider))
	root.AddCommand(commands.NewHistoryCommand(toolProvider))
	root.AddCommand(commands.NewBranchesContainingCommand(toolProvider))
	return root
}
