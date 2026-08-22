package commands

import (
	"github.com/autonomous-bits/spool/internal/resolve"
	"github.com/spf13/cobra"
)

// NewGraphCommand creates a read-only complete snapshot export command.
func NewGraphCommand(toolProvider func() (*resolve.ResolveTool, error)) *cobra.Command {
	var branch string
	command := &cobra.Command{
		Use:          "graph",
		Short:        "Export all nodes and edges from a branch snapshot",
		Long:         "Return the complete immutable graph snapshot as JSON for visualization and offline inspection.",
		Example:      "  spl graph --branch main",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			tool, err := toolProvider()
			if err != nil {
				return err
			}
			result, err := tool.EDGGraph(command.Context(), resolve.SnapshotSelector{Branch: branch})
			if err != nil {
				return err
			}
			return writeJSON(command, result, "graph")
		},
	}
	command.Flags().StringVar(&branch, "branch", "", "branch to export")
	_ = command.MarkFlagRequired("branch")
	return command
}
