package commands

import (
	"github.com/autonomous-bits/spool/internal/resolve"
	"github.com/spf13/cobra"
)

// NewFilterCommand creates the spl filter command.
func NewFilterCommand(toolProvider func() (*resolve.ResolveTool, error)) *cobra.Command {
	var branch, commit, token string
	var filters retrievalFilterFlags
	var budgetFlags queryBudgetFlags
	command := &cobra.Command{
		Use:          "filter",
		Short:        "Filter nodes by labels and indexed properties",
		Long:         "Return JSON nodes selected by labels and typed indexed-property filters. SQL and projection query syntax are not accepted.",
		Example:      "  spl filter --branch main --label Task --property-text status=open\n  spl filter --branch main --property-min priority=3",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			predicates, err := filters.predicates()
			if err != nil {
				return err
			}
			tool, err := toolProvider()
			if err != nil {
				return err
			}
			result, err := tool.SPLFilter(command.Context(), resolve.FilterRequest{
				Selector:          snapshotSelectorFlag(command, "commit", branch, commit),
				Labels:            filters.labels,
				Predicates:        predicates,
				ContinuationToken: token,
				Budget:            budgetFlags.request(command),
			})
			if err != nil {
				return err
			}
			return writeJSON(command, result, "filter")
		},
	}
	command.Flags().StringVar(&branch, "branch", "", "branch-head projection to query")
	command.Flags().StringVar(&commit, "commit", "", "commit selector; only the current branch head is supported")
	filters.add(command)
	budgetFlags.addPagedQueryFlags(command, &token)
	_ = command.MarkFlagRequired("branch")
	return command
}
