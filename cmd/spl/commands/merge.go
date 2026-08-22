package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

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
	command.AddCommand(newMergeConflictsCommand(toolProvider))
	command.AddCommand(newMergeResolveCommand(toolProvider))
	command.AddCommand(newMergeFinalizeCommand(toolProvider))
	command.AddCommand(newMergeAbortCommand(toolProvider))
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
			result, err := tool.SPLMergePreview(command.Context(), source, target)
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
			commit, err := tool.SPLApplyMergePreview(command.Context(), resolve.MergeApplyRequest{
				SourceBranch: source, TargetBranch: target, TransactionID: transactionID,
				PreviewID: repository.ObjectID(previewID), Author: author, Message: message,
			})
			if err != nil {
				if errors.Is(err, repository.ErrMergeConflicted) {
					result, inspectErr := tool.SPLMergeConflicts(command.Context(), resolve.MergeConflictsRequest{
						TargetBranch: target, TransactionID: transactionID,
					})
					if inspectErr != nil {
						return inspectErr
					}
					return json.NewEncoder(command.OutOrStdout()).Encode(result)
				}
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

func newMergeConflictsCommand(toolProvider func() (*resolve.ResolveTool, error)) *cobra.Command {
	var target, transactionID string
	command := &cobra.Command{
		Use:          "conflicts",
		Short:        "Inspect a persisted conflicted merge",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			tool, err := toolProvider()
			if err != nil {
				return err
			}

			result, err := tool.SPLMergeConflicts(command.Context(), resolve.MergeConflictsRequest{
				TargetBranch: target, TransactionID: transactionID,
			})
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(result)
		},
	}
	command.Flags().StringVar(&target, "target", "", "target branch with the conflicted merge")
	command.Flags().StringVar(&transactionID, "transaction", "", "owning merge transaction identifier")
	_ = command.MarkFlagRequired("target")
	_ = command.MarkFlagRequired("transaction")
	return command
}

func newMergeFinalizeCommand(toolProvider func() (*resolve.ResolveTool, error)) *cobra.Command {
	var target, transactionID string
	command := &cobra.Command{Use: "finalize", Short: "Finalize a resolved conflicted merge", Args: cobra.NoArgs, SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			tool, err := toolProvider()
			if err != nil {
				return err
			}
			commit, err := tool.SPLFinalizeMerge(command.Context(), resolve.MergeTransactionRequest{TargetBranch: target, TransactionID: transactionID})
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(struct {
				Commit repository.ObjectID `json:"commit"`
			}{Commit: commit})
		}}
	command.Flags().StringVar(&target, "target", "", "target branch with the conflicted merge")
	command.Flags().StringVar(&transactionID, "transaction", "", "owning merge transaction identifier")
	_ = command.MarkFlagRequired("target")
	_ = command.MarkFlagRequired("transaction")
	return command
}

func newMergeResolveCommand(toolProvider func() (*resolve.ResolveTool, error)) *cobra.Command {
	var target, transactionID, previewID, selectionsPath, overridesPath string
	command := &cobra.Command{Use: "resolve", Short: "Resolve every conflict in a persisted merge", Args: cobra.NoArgs, SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			selectionsData, err := os.ReadFile(selectionsPath)
			if err != nil {
				return fmt.Errorf("read merge selections: %w", err)
			}
			var selections []repository.MergeResolutionSelection
			if err := json.Unmarshal(selectionsData, &selections); err != nil {
				return fmt.Errorf("decode merge selections: %w", err)
			}
			var overrides []repository.MutationOperation
			if overridesPath != "" {
				overridesData, err := os.ReadFile(overridesPath)
				if err != nil {
					return fmt.Errorf("read merge overrides: %w", err)
				}
				if err := json.Unmarshal(overridesData, &overrides); err != nil {
					return fmt.Errorf("decode merge overrides: %w", err)
				}
			}
			tool, err := toolProvider()
			if err != nil {
				return err
			}
			if err := tool.SPLResolveMerge(command.Context(), resolve.MergeResolveRequest{
				TargetBranch: target, TransactionID: transactionID, PreviewID: repository.ObjectID(previewID),
				Selections: selections, Overrides: overrides,
			}); err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(struct {
				Resolved bool `json:"resolved"`
			}{Resolved: true})
		}}
	command.Flags().StringVar(&target, "target", "", "target branch with the conflicted merge")
	command.Flags().StringVar(&transactionID, "transaction", "", "owning merge transaction identifier")
	command.Flags().StringVar(&previewID, "preview", "", "persisted preview identifier")
	command.Flags().StringVar(&selectionsPath, "selections", "", "path to a JSON conflict-selection array")
	command.Flags().StringVar(&overridesPath, "overrides", "", "path to an optional JSON mutation-operation array")
	for _, name := range []string{"target", "transaction", "preview", "selections"} {
		_ = command.MarkFlagRequired(name)
	}
	return command
}

func newMergeAbortCommand(toolProvider func() (*resolve.ResolveTool, error)) *cobra.Command {
	var target, transactionID string
	command := &cobra.Command{Use: "abort", Short: "Abort a conflicted merge", Args: cobra.NoArgs, SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			tool, err := toolProvider()
			if err != nil {
				return err
			}
			if err := tool.SPLAbortMerge(command.Context(), resolve.MergeTransactionRequest{TargetBranch: target, TransactionID: transactionID}); err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(struct {
				Aborted bool `json:"aborted"`
			}{Aborted: true})
		}}
	command.Flags().StringVar(&target, "target", "", "target branch with the conflicted merge")
	command.Flags().StringVar(&transactionID, "transaction", "", "owning merge transaction identifier")
	_ = command.MarkFlagRequired("target")
	_ = command.MarkFlagRequired("transaction")
	return command
}
