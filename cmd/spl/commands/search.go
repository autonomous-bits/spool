package commands

import (
	"github.com/autonomous-bits/spool/internal/ctxgit"
	"github.com/spf13/cobra"
)

// NewSearchCommand creates the bound-only search command.
func NewSearchCommand(opts ctxgit.Options) *cobra.Command {
	var query string
	command := &cobra.Command{
		Use:          "search",
		Short:        "Search nodes lexically in the bound context checkout",
		Long:         "Return JSON lexical matches from the rebuilt context-git projection. Unbound workspaces are refused.",
		Example:      "  spl search --query incident",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			session, err := startBoundSession(command, opts)
			if err != nil {
				return err
			}
			result, err := session.SearchResult(command.Context(), query, 20)
			if err != nil {
				return err
			}
			return writeJSON(command, result, "search")
		},
	}
	command.Flags().StringVar(&query, "query", "", "lexical query")
	_ = command.MarkFlagRequired("query")
	return command
}
