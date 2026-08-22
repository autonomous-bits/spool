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
	}, func() (*resolve.FsckTool, error) {
		return tool.FsckTool(), nil
	})
}

func newRootCommandWithLifecycle(
	stdout io.Writer,
	toolProvider func() (*resolve.ResolveTool, error),
	initialize func() (*repository.Repository, error),
	fsckProviders ...func() (*resolve.FsckTool, error),
) *cobra.Command {
	fsckProvider := func() (*resolve.FsckTool, error) {
		tool, err := toolProvider()
		if err != nil {
			return nil, err
		}
		return tool.FsckTool(), nil
	}
	if len(fsckProviders) > 0 {
		fsckProvider = fsckProviders[0]
	}
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
	root.AddCommand(commands.NewSchemaCommand(toolProvider))
	root.AddCommand(commands.NewValidateCommand(toolProvider))
	root.AddCommand(commands.NewDiffCommand(toolProvider))
	root.AddCommand(commands.NewHistoryCommand(toolProvider))
	root.AddCommand(commands.NewBranchesContainingCommand(toolProvider))
	root.AddCommand(commands.NewFilterCommand(toolProvider))
	root.AddCommand(commands.NewSearchCommand(toolProvider))
	root.AddCommand(commands.NewSearchExpandCommand(toolProvider))
	root.AddCommand(commands.NewContextCommand(toolProvider))
	root.AddCommand(commands.NewGraphCommand(toolProvider))
	root.AddCommand(commands.NewMergeCommand(toolProvider))
	root.AddCommand(commands.NewFsckCommand(fsckProvider))
	root.AddCommand(commands.NewGCCommand(toolProvider))
	return root
}
