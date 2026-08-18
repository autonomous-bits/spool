package commands

import (
	"encoding/json"
	"fmt"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/resolve"
	"github.com/spf13/cobra"
)

// NewDiffCommand creates the command for comparing two branch or commit snapshots.
func NewDiffCommand(toolProvider func() (*resolve.ResolveTool, error)) *cobra.Command {
	var baseBranch, baseCommit, targetBranch, targetCommit, title, token string
	var nodeIDs, edgeIDs []string
	var maxRows, maxResponseBytes int
	var oneHop bool
	command := &cobra.Command{
		Use:          "diff",
		Short:        "Compare two graph snapshots",
		Long:         "Compare snapshots selected by branches or commits. Filters, pagination, one-hop context, and response budgets constrain the JSON result.",
		Example:      "  spl diff --base-branch main --target-branch feature\n  spl diff --base-commit <base-id> --target-commit <target-id> --max-rows 100",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			tool, err := toolProvider()
			if err != nil {
				return err
			}
			result, err := tool.EDGDiff(command.Context(), resolve.DiffRequest{
				Base:          repository.DiffSelector{Branch: baseBranch, Commit: baseCommit},
				Target:        repository.DiffSelector{Branch: targetBranch, Commit: targetCommit},
				Filter:        repository.DiffFilter{NodeIDs: nodeIDs, EdgeIDs: edgeIDs, NodeTitleSubstr: title},
				IncludeOneHop: oneHop, ContinuationToken: token,
				Budget: diffBudget(command, &maxRows, &maxResponseBytes),
			})
			if err != nil {
				return err
			}
			data, err := json.Marshal(result)
			if err != nil {
				return err
			}
			if _, err := command.OutOrStdout().Write(data); err != nil {
				return fmt.Errorf("write diff result: %w", err)
			}
			return nil
		},
	}
	command.Flags().StringVar(&baseBranch, "base-branch", "", "base branch selector")
	command.Flags().StringVar(&baseCommit, "base-commit", "", "base commit selector")
	command.Flags().StringVar(&targetBranch, "target-branch", "", "target branch selector")
	command.Flags().StringVar(&targetCommit, "target-commit", "", "target commit selector")
	command.Flags().StringSliceVar(&nodeIDs, "node-id", nil, "node ID filter")
	command.Flags().StringSliceVar(&edgeIDs, "edge-id", nil, "edge ID filter")
	command.Flags().StringVar(&title, "node-title-contains", "", "node title substring filter")
	command.Flags().BoolVar(&oneHop, "one-hop", false, "include one-hop context")
	command.Flags().StringVar(&token, "continuation", "", "continuation token")
	command.Flags().IntVar(&maxRows, "max-rows", 0, "maximum rows to return")
	command.Flags().IntVar(&maxResponseBytes, "max-response-bytes", 0, "maximum response size in bytes")
	return command
}

// diffBudget returns only the diff budget limits explicitly supplied on the command line.
func diffBudget(command *cobra.Command, rows, bytes *int) resolve.QueryBudgetRequest {
	budget := resolve.QueryBudgetRequest{}
	if command.Flags().Changed("max-rows") {
		budget.MaxRows = rows
	}
	if command.Flags().Changed("max-response-bytes") {
		budget.MaxResponseBytes = bytes
	}
	return budget
}
