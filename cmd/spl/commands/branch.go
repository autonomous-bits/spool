package commands

import (
	"encoding/json"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/spf13/cobra"
)

// NewBranchCommand creates the branch command with a repository provider.
func NewBranchCommand(repoProvider func() (*repository.Repository, error)) *cobra.Command {
	var sourceBranch, sourceCommit string
	command := &cobra.Command{
		Use:          "branch",
		Short:        "Manage local branches",
		Long:         "Create, list, and delete local branches. Branch creation must name exactly one existing branch or commit as its source.",
		Example:      "  spl branch create feature --from-branch main\n  spl branch list\n  spl branch delete feature",
		SilenceUsage: true,
	}
	create := &cobra.Command{
		Use:          "create <name>",
		Short:        "Create a branch at an explicit source",
		Long:         "Create a branch from exactly one existing branch or commit. The result is written as JSON to standard output.",
		Example:      "  spl branch create feature --from-branch main\n  spl branch create review --from-commit <commit-id>",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(command *cobra.Command, args []string) error {
			repo, err := repoProvider()
			if err != nil {
				return err
			}
			result, err := repo.CreateBranch(args[0], repository.BranchSource{
				Branch: sourceBranch,
				Commit: sourceCommit,
			})
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(result)
		},
	}
	create.Flags().StringVar(&sourceBranch, "from-branch", "", "existing branch to use as the source")
	create.Flags().StringVar(&sourceCommit, "from-commit", "", "existing commit to use as the source")
	list := &cobra.Command{
		Use:          "list",
		Short:        "List local branches",
		Long:         "List local branch names in deterministic order as JSON.",
		Example:      "  spl branch list",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			repo, err := repoProvider()
			if err != nil {
				return err
			}
			result, err := repo.ListBranches()
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(result)
		},
	}
	deleteBranch := &cobra.Command{
		Use:          "delete <name>",
		Short:        "Delete a local branch",
		Long:         "Delete an inactive, non-default local branch and write the deletion result as JSON.",
		Example:      "  spl branch delete feature",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(command *cobra.Command, args []string) error {
			repo, err := repoProvider()
			if err != nil {
				return err
			}
			result, err := repo.DeleteBranch(args[0])
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(result)
		},
	}
	command.AddCommand(create, list, deleteBranch)
	return command
}
