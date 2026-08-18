package commands

import (
	"encoding/json"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/resolve"
	"github.com/spf13/cobra"
)

// NewHistoryCommand creates the command for retrieving an entity's commit history.
func NewHistoryCommand(toolProvider func() (*resolve.ResolveTool, error)) *cobra.Command {
	var branch, commit, entity string
	var allParents bool
	command := &cobra.Command{
		Use:          "history",
		Short:        "Return an entity's commit history",
		Long:         "Return commits that changed an entity, starting from a branch or commit selector. Use --all-parents to traverse every merge parent.",
		Example:      "  spl history --branch main --entity-id <node-id>\n  spl history --commit <commit-id> --entity-id <node-id> --all-parents",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			tool, err := toolProvider()
			if err != nil {
				return err
			}
			result, err := tool.EDGHistory(command.Context(), resolve.HistoryRequest{
				Selector: repository.DiffSelector{Branch: branch, Commit: commit}, EntityID: entity, AllParents: allParents,
			})
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(result)
		},
	}
	command.Flags().StringVar(&branch, "branch", "", "branch at which to start history traversal")
	command.Flags().StringVar(&commit, "commit", "", "commit at which to start history traversal")
	command.Flags().StringVar(&entity, "entity-id", "", "stable entity ID")
	command.Flags().BoolVar(&allParents, "all-parents", false, "traverse all merge parents")
	_ = command.MarkFlagRequired("entity-id")
	return command
}

// NewBranchesContainingCommand creates the command for finding branches that contain a selector.
func NewBranchesContainingCommand(toolProvider func() (*resolve.ResolveTool, error)) *cobra.Command {
	var entity, snapshot, naturalKey string
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
			result, err := tool.EDGBranchesContaining(command.Context(), resolve.ContainmentSelector{
				EntityID: entity, SnapshotID: repository.ObjectID(snapshot), NaturalKey: naturalKey,
			})
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(result)
		},
	}
	command.Flags().StringVar(&entity, "entity-id", "", "stable entity ID")
	command.Flags().StringVar(&snapshot, "snapshot-id", "", "exact snapshot object ID")
	command.Flags().StringVar(&naturalKey, "natural-key", "", "schema-defined natural key")
	return command
}
