package commands

import (
	"encoding/json"

	"github.com/autonomous-bits/spool/internal/ctxgit"
	"github.com/spf13/cobra"
)

// NewValidateCommand validates the bound context checkout against schema.toml.
func NewValidateCommand(opts ctxgit.Options) *cobra.Command {
	command := &cobra.Command{
		Use:          "validate",
		Short:        "Validate the bound context graph against its schema",
		Long:         "Validate the bound context-git checkout against schema.toml. Unbound workspaces are refused.",
		Example:      "  spl validate",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			session, err := startBoundSession(command, opts)
			if err != nil {
				return err
			}
			result, err := session.ValidateSchema()
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(result)
		},
	}
	return command
}
