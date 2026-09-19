package main

import (
	"io"

	"github.com/autonomous-bits/spool/cmd/spl/commands"
	"github.com/autonomous-bits/spool/internal/ctxgit"
	"github.com/spf13/cobra"
)

func newRootCommand(stdout io.Writer) *cobra.Command {
	return newRootCommandWithOptions(stdout, ctxgit.Options{})
}

func newRootCommandWithOptions(stdout io.Writer, opts ctxgit.Options) *cobra.Command {
	root := &cobra.Command{
		Use:          "spl",
		Short:        "Spool solution-context CLI (bound context git)",
		SilenceUsage: true,
	}
	root.SetOut(stdout)
	root.AddCommand(commands.NewContextCommand(opts))
	root.AddCommand(commands.NewMCPCommand(opts))
	root.AddCommand(commands.NewVersionCommand())
	root.AddCommand(commands.NewResolveCommand(opts))
	root.AddCommand(commands.NewSearchCommand(opts))
	root.AddCommand(commands.NewSearchExpandCommand(opts))
	root.AddCommand(commands.NewQueryContextCommand(opts))
	root.AddCommand(commands.NewFilterCommand(opts))
	root.AddCommand(commands.NewGraphCommand(opts))
	root.AddCommand(commands.NewSchemaCommand(opts))
	root.AddCommand(commands.NewValidateCommand(opts))
	root.AddCommand(commands.NewAssetCommand(opts))
	root.AddCommand(commands.NewMergeCommand(opts))
	root.AddCommand(commands.NewPruneCommand(opts))
	return root
}
