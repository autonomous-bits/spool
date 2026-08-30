package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/spf13/cobra"
)

// NewMergeCommand creates the command group for previewing and applying graph merges.
func NewMergeCommand(repoProvider func() (*repository.Repository, error)) *cobra.Command {
	command := &cobra.Command{
		Use:          "merge",
		Short:        "Preview and apply three-way graph merges",
		SilenceUsage: true,
	}
	command.AddCommand(newMergePreviewCommand(repoProvider))
	command.AddCommand(newMergeApplyCommand(repoProvider))
	command.AddCommand(newMergeConflictsCommand(repoProvider))
	command.AddCommand(newMergeResolveCommand(repoProvider))
	command.AddCommand(newMergeFinalizeCommand(repoProvider))
	command.AddCommand(newMergeAbortCommand(repoProvider))
	return command
}

func newMergePreviewCommand(repoProvider func() (*repository.Repository, error)) *cobra.Command {
	var source, target string
	command := &cobra.Command{
		Use:          "preview",
		Short:        "Compute a deterministic three-way merge preview",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			repo, err := repoProvider()
			if err != nil {
				return err
			}
			result, err := repo.PreviewMerge(source, target)
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

func newMergeApplyCommand(repoProvider func() (*repository.Repository, error)) *cobra.Command {
	var source, target, transactionID, previewID, author, message string
	command := &cobra.Command{
		Use:          "apply",
		Short:        "Apply an exact clean merge preview",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			repo, err := repoProvider()
			if err != nil {
				return err
			}
			commit, err := repo.ApplyMergePreview(source, target, transactionID, repository.ObjectID(previewID), author, message)
			if err != nil {
				if errors.Is(err, repository.ErrMergeConflicted) {
					result, inspectErr := repo.InspectMergeTransaction(target, transactionID)
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

func newMergeConflictsCommand(repoProvider func() (*repository.Repository, error)) *cobra.Command {
	var target, transactionID string
	command := &cobra.Command{
		Use:          "conflicts",
		Short:        "Inspect a persisted conflicted merge",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			repo, err := repoProvider()
			if err != nil {
				return err
			}

			result, err := repo.InspectMergeTransaction(target, transactionID)
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

func newMergeFinalizeCommand(repoProvider func() (*repository.Repository, error)) *cobra.Command {
	var target, transactionID string
	command := &cobra.Command{Use: "finalize", Short: "Finalize a resolved conflicted merge", Args: cobra.NoArgs, SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			repo, err := repoProvider()
			if err != nil {
				return err
			}
			commit, err := repo.FinalizeMergeTransaction(target, transactionID)
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

func newMergeResolveCommand(repoProvider func() (*repository.Repository, error)) *cobra.Command {
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
			repo, err := repoProvider()
			if err != nil {
				return err
			}
			if err := repo.ResolveConflictedMerge(repository.ResolveConflictedMergeRequest{
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

func newMergeAbortCommand(repoProvider func() (*repository.Repository, error)) *cobra.Command {
	var target, transactionID string
	command := &cobra.Command{Use: "abort", Short: "Abort a conflicted merge", Args: cobra.NoArgs, SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			repo, err := repoProvider()
			if err != nil {
				return err
			}
			if err := repo.AbortMergeTransaction(target, transactionID); err != nil {
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
