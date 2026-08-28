package commands

import (
	"encoding/json"
	"errors"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/resolve"
	"github.com/spf13/cobra"
)

// NewPruneCommand creates the prune command for excising ephemeral entities and cascading edges.
func NewPruneCommand(toolProvider func() (*resolve.ResolveTool, error)) *cobra.Command {
	var (
		branch  string
		dryRun  bool
		force   bool
		author  string
		message string
	)
	command := &cobra.Command{
		Use:          "prune",
		Short:        "Prune ephemeral entities and cascading relationships",
		Long:         "Prune temporary planning entities (nodes marked with the Ephemeral modifier label) and their cascading incident relationships from a branch prior to baseline merge. The result is emitted as machine-readable JSON.",
		Example:      "  spl prune --branch feature/login\n  spl prune --branch feature/login --dry-run\n  spl prune --branch main --force",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := command.Context().Err(); err != nil {
				return err
			}
			tool, err := toolProvider()
			if err != nil {
				return err
			}
			result, pruneErr := tool.SPLPrune(command.Context(), repository.PruneRequest{
				Branch:  branch,
				DryRun:  dryRun,
				Force:   force,
				Author:  author,
				Message: message,
			})
			if pruneErr != nil {
				var warning *repository.PruneCommittedWithWarningError
				if !errors.As(pruneErr, &warning) {
					return pruneErr
				}
			}
			if err := json.NewEncoder(command.OutOrStdout()).Encode(result); err != nil {
				return err
			}
			return pruneErr
		},
	}
	command.Flags().StringVar(&branch, "branch", "", "branch whose ephemeral entities to prune")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "simulate pruning and report affected entities without writing changes")
	command.Flags().BoolVar(&force, "force", false, "allow pruning on protected default branch")
	command.Flags().StringVar(&author, "author", "", "override commit author")
	command.Flags().StringVar(&message, "message", "", "override commit message")
	_ = command.MarkFlagRequired("branch")
	return command
}
