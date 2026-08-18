package commands

import (
	"encoding/json"
	"errors"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/resolve"
	"github.com/spf13/cobra"
)

// NewCommitCommand creates the command for committing a branch's current staged mutation set.
func NewCommitCommand(toolProvider func() (*resolve.ResolveTool, error)) *cobra.Command {
	var branchName, author, message string
	command := &cobra.Command{
		Use:          "commit",
		Short:        "Commit a branch's staged mutation delta",
		Long:         "Commit all staged graph mutations for a branch. The result is written as JSON to standard output; a committed durability warning is returned after the result.",
		Example:      "  spl commit --branch main --author alice --message 'Add a requirement node'",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			tool, err := toolProvider()
			if err != nil {
				return err
			}
			result, err := tool.EDGCommitStagedMutationBatch(command.Context(), repository.CommitStagedMutationRequest{
				Branch: branchName, Author: author, Message: message,
			})
			if err != nil {
				var warning *repository.CommittedWithWarningError
				if !errors.As(err, &warning) {
					return err
				}
				if encodeErr := json.NewEncoder(command.OutOrStdout()).Encode(result); encodeErr != nil {
					return encodeErr
				}
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(result)
		},
	}
	command.Flags().StringVar(&branchName, "branch", "", "branch whose staged mutations to commit")
	command.Flags().StringVar(&author, "author", "", "commit author")
	command.Flags().StringVar(&message, "message", "", "commit message")
	_ = command.MarkFlagRequired("branch")
	return command
}
