package commands

import (
	"github.com/autonomous-bits/spool/internal/resolve"
	"github.com/spf13/cobra"
)

// NewSearchCommand creates the spl search command.
func NewSearchCommand(toolProvider func() (*resolve.ResolveTool, error)) *cobra.Command {
	var branch, commit, query, token string
	var budgetFlags queryBudgetFlags
	command := &cobra.Command{
		Use:          "search",
		Short:        "Search nodes lexically",
		Long:         "Return JSON lexical matches from the branch-head projection.",
		Example:      "  spl search --branch main --query incident",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			tool, err := toolProvider()
			if err != nil {
				return err
			}
			result, err := tool.SPLSearch(command.Context(), resolve.SearchRequest{
				Selector:          snapshotSelectorFlag(command, "commit", branch, commit),
				Query:             query,
				ContinuationToken: token,
				Budget:            budgetFlags.request(command),
			})
			if err != nil {
				return err
			}
			return writeJSON(command, result, "search")
		},
	}
	command.Flags().StringVar(&branch, "branch", "", "branch-head projection to query")
	command.Flags().StringVar(&commit, "commit", "", "commit selector; only the current branch head is supported")
	command.Flags().StringVar(&query, "query", "", "lexical query")
	budgetFlags.addPagedQueryFlags(command, &token)
	_ = command.MarkFlagRequired("branch")
	_ = command.MarkFlagRequired("query")
	return command
}
