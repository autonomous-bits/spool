package commands

import (
	"context"
	"encoding/json"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/spf13/cobra"
)

// NewFsckCommand creates the command that verifies durable repository integrity.
func NewFsckCommand(fsckProvider func(context.Context) (repository.FsckResult, error)) *cobra.Command {
	return &cobra.Command{
		Use:          "fsck",
		Short:        "Verify repository object and graph integrity",
		Long:         "Read-only traversal of branch references, loose objects, snapshots, graph projections, schemas, staged state, and merge bindings. The complete JSON report is written even when corruption is found; corruption returns a non-zero status and is not repaired automatically.",
		Example:      "  spl fsck",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			result, fsckErr := fsckProvider(command.Context())
			if err := json.NewEncoder(command.OutOrStdout()).Encode(result); err != nil {
				return err
			}
			return fsckErr
		},
	}
}
