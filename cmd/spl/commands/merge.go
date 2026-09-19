package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/autonomous-bits/spool/internal/ctxgit"
	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/spf13/cobra"
)

// NewMergeCommand creates file-graph merge commands for bound context git.
func NewMergeCommand(opts ctxgit.Options) *cobra.Command {
	command := &cobra.Command{
		Use:          "merge",
		Short:        "Preview and apply three-way file-graph merges",
		Long:         "Merge context-git branches as file-level node/edge mutations. Clean applies open a short-lived branch and PR. This is not a Rack/.spl merge lease.",
		SilenceUsage: true,
	}
	command.AddCommand(newMergePreviewCommand(opts))
	command.AddCommand(newMergeApplyCommand(opts))
	command.AddCommand(newMergeConflictsCommand(opts))
	command.AddCommand(newMergeResolveCommand(opts))
	command.AddCommand(newMergeFinalizeCommand(opts))
	command.AddCommand(newMergeAbortCommand(opts))
	return command
}

func newMergePreviewCommand(opts ctxgit.Options) *cobra.Command {
	var source, target string
	command := &cobra.Command{
		Use:          "preview",
		Short:        "Compute a deterministic three-way file-graph merge preview",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			session, err := startBoundSession(command, opts)
			if err != nil {
				return err
			}
			result, err := session.PreviewMerge(command.Context(), source, target)
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(result)
		},
	}
	command.Flags().StringVar(&source, "source", "", "source git branch")
	command.Flags().StringVar(&target, "target", "", "target git branch")
	_ = command.MarkFlagRequired("source")
	_ = command.MarkFlagRequired("target")
	return command
}

func newMergeApplyCommand(opts ctxgit.Options) *cobra.Command {
	var source, target, transactionID, previewID, author, message string
	command := &cobra.Command{
		Use:          "apply",
		Short:        "Apply a clean file-graph merge preview via branch + PR",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			session, err := startBoundSession(command, opts)
			if err != nil {
				return err
			}
			write, status, err := session.ApplyMerge(command.Context(), source, target, transactionID, previewID, author, message)
			if err != nil {
				if errors.Is(err, repository.ErrMergeConflicted) {
					return json.NewEncoder(command.OutOrStdout()).Encode(status)
				}
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(write)
		},
	}
	command.Flags().StringVar(&source, "source", "", "source git branch")
	command.Flags().StringVar(&target, "target", "", "target git branch")
	command.Flags().StringVar(&transactionID, "transaction", "", "caller transaction identifier")
	command.Flags().StringVar(&previewID, "preview", "", "deterministic preview identifier")
	command.Flags().StringVar(&author, "author", "", "merge commit author")
	command.Flags().StringVar(&message, "message", "", "merge commit message")
	for _, name := range []string{"source", "target", "transaction", "preview"} {
		_ = command.MarkFlagRequired(name)
	}
	return command
}

func newMergeConflictsCommand(opts ctxgit.Options) *cobra.Command {
	var transactionID string
	command := &cobra.Command{
		Use:          "conflicts",
		Short:        "Inspect a persisted conflicted file-graph merge",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			session, err := startBoundSession(command, opts)
			if err != nil {
				return err
			}
			result, err := session.InspectMerge(transactionID)
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(result)
		},
	}
	command.Flags().StringVar(&transactionID, "transaction", "", "owning merge transaction identifier")
	_ = command.MarkFlagRequired("transaction")
	return command
}

func newMergeFinalizeCommand(opts ctxgit.Options) *cobra.Command {
	var transactionID, author, message string
	command := &cobra.Command{
		Use: "finalize", Short: "Finalize a resolved conflicted file-graph merge", Args: cobra.NoArgs, SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			session, err := startBoundSession(command, opts)
			if err != nil {
				return err
			}
			write, err := session.FinalizeMerge(command.Context(), transactionID, author, message)
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(write)
		},
	}
	command.Flags().StringVar(&transactionID, "transaction", "", "owning merge transaction identifier")
	command.Flags().StringVar(&author, "author", "", "git author")
	command.Flags().StringVar(&message, "message", "", "commit/PR message")
	_ = command.MarkFlagRequired("transaction")
	return command
}

func newMergeResolveCommand(opts ctxgit.Options) *cobra.Command {
	var transactionID, previewID, selectionsPath, overridesPath string
	command := &cobra.Command{
		Use: "resolve", Short: "Resolve every conflict in a persisted file-graph merge", Args: cobra.NoArgs, SilenceUsage: true,
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
			session, err := startBoundSession(command, opts)
			if err != nil {
				return err
			}
			if err := session.ResolveMerge(command.Context(), transactionID, previewID, selections, overrides); err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(struct {
				Resolved bool `json:"resolved"`
			}{Resolved: true})
		},
	}
	command.Flags().StringVar(&transactionID, "transaction", "", "owning merge transaction identifier")
	command.Flags().StringVar(&previewID, "preview", "", "persisted preview identifier")
	command.Flags().StringVar(&selectionsPath, "selections", "", "path to a JSON conflict-selection array")
	command.Flags().StringVar(&overridesPath, "overrides", "", "path to an optional JSON mutation-operation array")
	for _, name := range []string{"transaction", "preview", "selections"} {
		_ = command.MarkFlagRequired(name)
	}
	return command
}

func newMergeAbortCommand(opts ctxgit.Options) *cobra.Command {
	var transactionID string
	command := &cobra.Command{
		Use: "abort", Short: "Abort a conflicted file-graph merge", Args: cobra.NoArgs, SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			session, err := startBoundSession(command, opts)
			if err != nil {
				return err
			}
			if err := session.AbortMerge(transactionID); err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(struct {
				Aborted bool `json:"aborted"`
			}{Aborted: true})
		},
	}
	command.Flags().StringVar(&transactionID, "transaction", "", "owning merge transaction identifier")
	_ = command.MarkFlagRequired("transaction")
	return command
}
