package main

import (
	"io"

	"github.com/autonomous-bits/spool/cmd/spl/commands"
	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/resolve"
	"github.com/spf13/cobra"
)

func newRootCommand(stdout io.Writer, repo *repository.Repository) *cobra.Command {
	tool := resolve.NewResolveTool(repo)
	return newRootCommandWithLifecycle(stdout, func() (*repository.Repository, error) {
		return repo, nil
	}, func() (*resolve.ResolveTool, error) {
		return tool, nil
	}, func() (*repository.Repository, error) {
		return repository.NewSeedRepository(), nil
	}, func() (*resolve.FsckTool, error) {
		return tool.FsckTool(), nil
	})
}

func newRootCommandWithLifecycle(
	stdout io.Writer,
	repoProvider func() (*repository.Repository, error),
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
	// state-dir is consumed manually in main() before the root command is
	// constructed (it determines which repository to open); it is declared
	// here only so cobra recognizes the flag instead of rejecting it.
	root.PersistentFlags().String("state-dir", "", "override the resolved Spool repository state directory")
	root.AddCommand(commands.NewInitCommand(initialize))
	root.AddCommand(commands.NewAddCommand(repoProvider))
	root.AddCommand(commands.NewStatusCommand(repoProvider))
	root.AddCommand(commands.NewCommitCommand(repoProvider))
	root.AddCommand(commands.NewBranchCommand(repoProvider))
	root.AddCommand(commands.NewSwitchCommand(repoProvider))
	root.AddCommand(commands.NewResolveCommand(toolProvider))
	root.AddCommand(commands.NewSchemaCommand(repoProvider))
	root.AddCommand(commands.NewValidateCommand(toolProvider))
	root.AddCommand(commands.NewDiffCommand(toolProvider))
	root.AddCommand(commands.NewHistoryCommand(toolProvider))
	root.AddCommand(commands.NewBranchesContainingCommand(toolProvider))
	root.AddCommand(commands.NewFilterCommand(toolProvider))
	root.AddCommand(commands.NewSearchCommand(toolProvider))
	root.AddCommand(commands.NewSearchExpandCommand(toolProvider))
	root.AddCommand(commands.NewContextCommand(toolProvider))
	root.AddCommand(commands.NewGraphCommand(toolProvider))
	root.AddCommand(commands.NewMergeCommand(repoProvider))
	root.AddCommand(commands.NewFsckCommand(fsckProvider))
	root.AddCommand(commands.NewGCCommand(repoProvider))
	root.AddCommand(commands.NewPruneCommand(repoProvider))
	root.AddCommand(commands.NewWorkspaceCommandDefault())
	return root
}
