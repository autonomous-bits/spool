package commands

import (
	"github.com/autonomous-bits/spool/internal/ctxgit"
	"github.com/spf13/cobra"
)

// NewGraphCommand creates a bound-only complete snapshot export command.
func NewGraphCommand(opts ctxgit.Options) *cobra.Command {
	command := &cobra.Command{
		Use:          "graph",
		Short:        "Export all nodes and edges from the bound context checkout",
		Long:         "Return the complete bound context-git graph as JSON. Unbound workspaces are refused.",
		Example:      "  spl graph",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			session, err := startBoundSession(command, opts)
			if err != nil {
				return err
			}
			result, err := session.GraphResult()
			if err != nil {
				return err
			}
			return writeJSON(command, result, "graph")
		},
	}
	return command
}
