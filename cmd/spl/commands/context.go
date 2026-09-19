package commands

import (
	"github.com/autonomous-bits/spool/internal/ctxgit"
	"github.com/spf13/cobra"
)

// NewContextCommand creates the spl context parent for init/export/migrate-once.
func NewContextCommand(opts ctxgit.Options) *cobra.Command {
	command := &cobra.Command{
		Use:          "context",
		Short:        "Bind and export solution context git",
		Long:         "Manage the explicit context-git bind. Querying the graph uses `spl query-context`.",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
	}
	command.AddCommand(NewContextInitCommand())
	command.AddCommand(NewContextExportCommand(opts))
	return command
}
