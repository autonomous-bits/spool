package commands

import (
	"encoding/json"

	"github.com/autonomous-bits/spool/internal/ctxgit"
	"github.com/spf13/cobra"
)

// NewPruneCommand prunes Ephemeral nodes on the bound context checkout.
func NewPruneCommand(opts ctxgit.Options) *cobra.Command {
	var dryRun bool
	var author, message string
	command := &cobra.Command{
		Use:          "prune",
		Short:        "Prune ephemeral entities from the bound context graph",
		Long:         "Prune temporary planning entities (nodes marked Ephemeral) and cascading incident edges from the bound context checkout. Writes a short-lived branch and pull request. Unbound workspaces are refused. This is not pack/CAS garbage collection.",
		Example:      "  spl prune --dry-run\n  spl prune --author alice --message \"Prune transient plan\"",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			session, err := startBoundSession(command, opts)
			if err != nil {
				return err
			}
			result, err := session.Prune(command.Context(), ctxgit.PruneRequest{
				DryRun:  dryRun,
				Author:  author,
				Message: message,
			})
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(result)
		},
	}
	command.Flags().BoolVar(&dryRun, "dry-run", false, "simulate pruning without writing a branch or PR")
	command.Flags().StringVar(&author, "author", "", "git author")
	command.Flags().StringVar(&message, "message", "", "commit/PR message")
	return command
}
