package commands

import (
	"encoding/json"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/resolve"
	"github.com/spf13/cobra"
)

// NewMergeCommand creates the command group for previewing and applying graph merges.
func NewMergeCommand(toolProvider func() (*resolve.ResolveTool, error)) *cobra.Command {
	command := &cobra.Command{
		Use:          "merge",
		Short:        "Preview and apply three-way graph merges",
		SilenceUsage: true,
	}
	command.AddCommand(newMergePreviewCommand(toolProvider))
	command.AddCommand(newMergeApplyCommand(toolProvider))
	return command
}

func newMergePreviewCommand(toolProvider func() (*resolve.ResolveTool, error)) *cobra.Command {
	var source, target string
	command := &cobra.Command{
		Use:          "preview",
		Short:        "Compute a deterministic three-way merge preview",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			tool, err := toolProvider()
			if err != nil {
				return err
			}
			result, err := tool.EDGMergePreview(command.Context(), source, target)
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(result)
		},
	}
	command.Flags().StringVar(&source, "source", "", "source branch to merge")
	command.Flags().StringVar(&target, "target", "", "target branch to update")
	_ = command.MarkFlagRequired("source")
	_ = command.MarkFlagRequired("target")
	return command
}

func newMergeApplyCommand(toolProvider func() (*resolve.ResolveTool, error)) *cobra.Command {
	var source, target, transactionID, previewID, author, message string
	command := &cobra.Command{
		Use:          "apply",
		Short:        "Apply an exact clean merge preview",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			tool, err := toolProvider()
			if err != nil {
				return err
			}
			commit, err := tool.EDGApplyMergePreview(command.Context(), resolve.MergeApplyRequest{
				SourceBranch: source, TargetBranch: target, TransactionID: transactionID,
				PreviewID: repository.ObjectID(previewID), Author: author, Message: message,
			})
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(struct {
				Commit repository.ObjectID `json:"commit"`
			}{Commit: commit})
		},
	}
	command.Flags().StringVar(&source, "source", "", "source branch from the reviewed preview")
	command.Flags().StringVar(&target, "target", "", "target branch from the reviewed preview")
	command.Flags().StringVar(&transactionID, "transaction", "", "caller transaction identifier")
	command.Flags().StringVar(&previewID, "preview", "", "deterministic preview identifier")
	command.Flags().StringVar(&author, "author", "", "merge commit author")
	command.Flags().StringVar(&message, "message", "", "merge commit message")
	for _, name := range []string{"source", "target", "transaction", "preview"} {
		_ = command.MarkFlagRequired(name)
	}
	return command
}
