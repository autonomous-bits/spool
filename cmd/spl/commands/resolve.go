// Package commands defines the spl CLI subcommands.
package commands

import (
	"encoding/json"
	"fmt"

	"github.com/autonomous-bits/spool/internal/resolve"
	"github.com/spf13/cobra"
)

// NewResolveCommand creates the resolve subcommand.
func NewResolveCommand(tool *resolve.ResolveTool) *cobra.Command {
	return NewResolveCommandWithToolProvider(func() (*resolve.ResolveTool, error) {
		return tool, nil
	})
}

// NewResolveCommandWithToolProvider creates the resolve command with a lazy repository tool.
func NewResolveCommandWithToolProvider(toolProvider func() (*resolve.ResolveTool, error)) *cobra.Command {
	var branch, commit, nodeID string
	var budgetFlags queryBudgetFlags
	command := &cobra.Command{
		Use:          "resolve",
		Short:        "Resolve a node from a branch snapshot",
		Long:         "Resolve a stable node ID from a branch snapshot or an explicitly selected reachable commit. The result includes snapshot and projection metadata as JSON.",
		Example:      "  spl resolve --branch main --node 11111111-1111-4111-8111-111111111111\n  spl resolve --branch main --commit <commit-id> --node <node-id>",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			if nodeID == "" {
				return fmt.Errorf("node is required")
			}
			tool, err := toolProvider()
			if err != nil {
				return err
			}
			selector := resolve.SnapshotSelector{Branch: branch}
			if command.Flags().Changed("commit") {
				selector.Commit = &commit
			}
			result, err := tool.SPLResolve(command.Context(), resolve.ResolveRequest{
				Selector: selector,
				NodeID:   nodeID,
				Budget:   budgetFlags.request(command),
			})
			if err != nil {
				return err
			}
			data, err := json.Marshal(result)
			if err != nil {
				return err
			}
			if _, err := command.OutOrStdout().Write(data); err != nil {
				return fmt.Errorf("write resolve result: %w", err)
			}
			return nil
		},
	}
	command.Flags().StringVar(&branch, "branch", "", "branch to resolve")
	command.Flags().StringVar(&commit, "commit", "", "commit to resolve")
	command.Flags().StringVar(&nodeID, "node", "", "stable node entity ID")
	budgetFlags.addReadBudgetFlags(command)
	budgetFlags.addTraversalBudgetFlags(command)
	_ = command.MarkFlagRequired("branch")
	return command
}
