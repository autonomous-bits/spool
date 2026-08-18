package commands

import (
	"encoding/json"

	"github.com/spf13/cobra"
	"github.com/autonomous-bits/spool/internal/repository"
)

// NewInitCommand creates the repository initialization command.
func NewInitCommand(initialize func() (*repository.Repository, error)) *cobra.Command {
	return &cobra.Command{
		Use:          "init",
		Short:        "Initialize a Spool repository",
		Long:         "Initialize the .spl state directory for the current workspace and create the default main branch.",
		Example:      "  spl init",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			repo, err := initialize()
			if err != nil {
				return err
			}
			result, err := repo.Initialization()
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(result)
		},
	}
}
