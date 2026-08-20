// Package commands defines the spl CLI subcommands.
package commands

import (
	"encoding/json"
	"fmt"
	"time"

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
	var maxRows, maxResponseBytes, maxDepth, maxVisited int
	var timeout time.Duration
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
			budget := resolve.QueryBudgetRequest{}
			if command.Flags().Changed("max-rows") {
				budget.MaxRows = &maxRows
			}
			if command.Flags().Changed("max-response-bytes") {
				budget.MaxResponseBytes = &maxResponseBytes
			}
			if command.Flags().Changed("max-depth") {
				budget.MaxDepth = &maxDepth
			}
			if command.Flags().Changed("max-visited") {
				budget.MaxVisited = &maxVisited
			}
			if command.Flags().Changed("timeout") {
				budget.Timeout = &timeout
			}
			result, err := tool.EDGResolve(command.Context(), resolve.ResolveRequest{
				Selector: selector,
				NodeID:   nodeID,
				Budget:   budget,
			})
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(result)
		},
	}
	command.Flags().StringVar(&branch, "branch", "", "branch to resolve")
	command.Flags().StringVar(&commit, "commit", "", "commit to resolve")
	command.Flags().StringVar(&nodeID, "node", "", "stable node entity ID")
	command.Flags().IntVar(&maxRows, "max-rows", 0, "maximum rows to return")
	command.Flags().IntVar(&maxResponseBytes, "max-response-bytes", 0, "maximum response size in bytes")
	command.Flags().IntVar(&maxDepth, "max-depth", 0, "maximum traversal depth")
	command.Flags().IntVar(&maxVisited, "max-visited", 0, "maximum visited nodes")
	command.Flags().DurationVar(&timeout, "timeout", 0, "maximum query duration")
	_ = command.MarkFlagRequired("branch")
	return command
}
