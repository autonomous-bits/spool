package commands

import (
	"encoding/json"
	"errors"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/spf13/cobra"
)

// NewMigrateCommand creates the repository format migration command.
func NewMigrateCommand(migrateProvider func(from, to int) (*repository.MigrationResult, error)) *cobra.Command {
	var fromVersion, toVersion int
	command := &cobra.Command{
		Use:          "migrate",
		Short:        "Migrate repository format version",
		Long:         "Upgrade an existing Spool repository state directory to a newer format version.",
		Example:      "  spl migrate --from 1 --to 2",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			if fromVersion <= 0 || toVersion <= 0 {
				return errors.New("both --from and --to flags are required (e.g. 'spl migrate --from 1 --to 2')")
			}
			if migrateProvider == nil {
				return errors.New("migration provider is not configured")
			}
			result, err := migrateProvider(fromVersion, toVersion)
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(result)
		},
	}
	command.Flags().IntVar(&fromVersion, "from", 0, "source repository format version")
	command.Flags().IntVar(&toVersion, "to", 0, "target repository format version")
	return command
}
