package commands

import (
	"encoding/json"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/spf13/cobra"
)

// NewStatusCommand creates the command for reporting a branch's staged delta.
func NewStatusCommand(repoProvider func() (*repository.Repository, error)) *cobra.Command {
	var branchName string
	command := &cobra.Command{
		Use:          "status",
		Short:        "Report a branch's staged mutation delta",
		Long:         "Report the staged graph-mutation delta for a branch as JSON. A branch with no staged changes returns an empty delta.",
		Example:      "  spl status --branch main",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			repo, err := repoProvider()
			if err != nil {
				return err
			}
			result, err := repo.BranchStagingStatus(branchName)
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(result)
		},
	}
	command.Flags().StringVar(&branchName, "branch", "", "branch whose staged delta to report")
	return command
}
