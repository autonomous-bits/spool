package commands

import (
	"encoding/json"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/spf13/cobra"
)

// NewSwitchCommand creates the command for switching the repository's active branch.
func NewSwitchCommand(repoProvider func() (*repository.Repository, error)) *cobra.Command {
	command := &cobra.Command{
		Use:          "switch <branch>",
		Short:        "Switch the active repository branch",
		Long:         "Update the repository's active branch pointer to an existing branch. Returns an error if the named branch does not exist.",
		Example:      "  spl switch feature\n  spl switch main",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(command *cobra.Command, args []string) error {
			repo, err := repoProvider()
			if err != nil {
				return err
			}
			result, err := repo.SwitchBranch(args[0])
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(result)
		},
	}
	return command
}
