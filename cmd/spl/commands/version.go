package commands

import (
	"encoding/json"

	"github.com/autonomous-bits/spool/internal/version"
	"github.com/spf13/cobra"
)

// NewVersionCommand creates the version display command.
func NewVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:          "version",
		Short:        "Print Spool version and build information",
		Long:         "Print Spool release version, commit SHA, build date, Go runtime version, and platform as JSON.",
		Example:      "  spl version",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			info := version.Get()
			return json.NewEncoder(command.OutOrStdout()).Encode(info)
		},
	}
}
