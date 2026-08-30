package commands

import (
	"encoding/json"
	"fmt"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/resolve"
	"github.com/spf13/cobra"
)

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
