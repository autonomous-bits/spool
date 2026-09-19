package commands

import (
	"github.com/autonomous-bits/spool/internal/ctxgit"
	"github.com/spf13/cobra"
)

// NewFilterCommand creates the bound-only filter command.
func NewFilterCommand(opts ctxgit.Options) *cobra.Command {
	var filters retrievalFilterFlags
	command := &cobra.Command{
		Use:          "filter",
		Short:        "Filter bound context nodes by labels and properties",
		Long:         "Return JSON nodes selected by labels and typed property filters from the bound checkout. Unbound workspaces are refused.",
		Example:      "  spl filter --label Task --property-text status=open",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			predicates, err := filters.predicates()
			if err != nil {
				return err
			}
			session, err := startBoundSession(command, opts)
			if err != nil {
				return err
			}
			result, err := session.FilterResult(filters.labels, predicates, 50)
			if err != nil {
				return err
			}
			return writeJSON(command, result, "filter")
		},
	}
	filters.add(command)
	return command
}
