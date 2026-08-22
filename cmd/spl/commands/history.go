package commands

import (
	"encoding/json"
	"fmt"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/resolve"
	"github.com/spf13/cobra"
)

// NewHistoryCommand creates the command for retrieving an entity's commit history.
func NewHistoryCommand(toolProvider func() (*resolve.ResolveTool, error)) *cobra.Command {
	var branch, commit, entity, token string
	var budgetFlags queryBudgetFlags
	var allParents bool
	command := &cobra.Command{
		Use:          "history",
		Short:        "Return an entity's commit history",
		Long:         "Return commits that changed an entity, starting from a branch and optional reachable commit. Use --all-parents to traverse every merge parent.",
		Example:      "  spl history --branch main --entity-id <node-id>\n  spl history --branch main --commit <commit-id> --entity-id <node-id> --all-parents",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			tool, err := toolProvider()
			if err != nil {
				return err
			}
			result, err := tool.SPLHistory(command.Context(), resolve.HistoryRequest{
				Selector: snapshotSelectorFlag(command, "commit", branch, commit), EntityID: entity, AllParents: allParents,
				ContinuationToken: token, Budget: budgetFlags.request(command),
			})
			if err != nil {
				return err
			}
			data, err := json.Marshal(result)
			if err != nil {
				return err
			}
			if _, err := command.OutOrStdout().Write(data); err != nil {
				return fmt.Errorf("write history result: %w", err)
			}
			return nil
		},
	}
	command.Flags().StringVar(&branch, "branch", "", "branch at which to start history traversal")
	command.Flags().StringVar(&commit, "commit", "", "commit at which to start history traversal")
	command.Flags().StringVar(&entity, "entity-id", "", "stable entity ID")
	command.Flags().BoolVar(&allParents, "all-parents", false, "traverse all merge parents")
	budgetFlags.addPagedQueryFlags(command, &token)
	_ = command.MarkFlagRequired("entity-id")
	_ = command.MarkFlagRequired("branch")
	return command
}

// NewBranchesContainingCommand creates the command for finding branches that contain a selector.
func NewBranchesContainingCommand(toolProvider func() (*resolve.ResolveTool, error)) *cobra.Command {
	var entity, snapshot, naturalKey, token string
	var budgetFlags queryBudgetFlags
	command := &cobra.Command{
		Use:          "branches-containing",
		Short:        "Find branches containing a selector",
		Long:         "Find branches containing an entity, exact snapshot, or schema-defined natural key. The result is written as JSON.",
		Example:      "  spl branches-containing --entity-id <node-id>\n  spl branches-containing --snapshot-id <snapshot-id>",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			tool, err := toolProvider()
			if err != nil {
				return err
			}
			result, err := tool.SPLBranchesContainingPage(command.Context(), resolve.BranchesContainingRequest{
				Selector: resolve.ContainmentSelector{
					EntityID: entity, SnapshotID: repository.ObjectID(snapshot), NaturalKey: naturalKey,
				},
				ContinuationToken: token,
				Budget:            budgetFlags.request(command),
			})
			if err != nil {
				return err
			}
			data, err := json.Marshal(result)
			if err != nil {
				return err
			}
			if _, err := command.OutOrStdout().Write(data); err != nil {
				return fmt.Errorf("write branches-containing result: %w", err)
			}
			return nil
		},
	}
	command.Flags().StringVar(&entity, "entity-id", "", "stable entity ID")
	command.Flags().StringVar(&snapshot, "snapshot-id", "", "exact snapshot object ID")
	command.Flags().StringVar(&naturalKey, "natural-key", "", "schema-defined natural key")
	budgetFlags.addPagedQueryFlags(command, &token)
	return command
}
