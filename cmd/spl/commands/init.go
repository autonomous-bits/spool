package commands

import (
	"encoding/json"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/spf13/cobra"
)

// NewInitCommand creates the repository initialization command.
func NewInitCommand(initialize func() (*repository.Repository, error)) *cobra.Command {
	return &cobra.Command{
		Use:          "init",
		Short:        "Initialize a Spool repository",
		Long:         "Initialize leftover local `.spl` state. Deprecated and unsupported for solution context: when `.spool/context.toml` is present this command is refused; use `spl context init --remote` instead. Leftover `.spl` is migration-only (`spl context export`).",
		Example:      "  spl init",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := refuseLegacyContextSoT("spl init"); err != nil {
				return err
			}
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
