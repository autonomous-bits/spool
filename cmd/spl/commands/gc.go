package commands

import (
	"encoding/json"
	"errors"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/resolve"
	"github.com/spf13/cobra"
)

// NewGCCommand creates the command that packs retained objects and prunes
// grace-expired unreachable loose objects.
func NewGCCommand(toolProvider func() (*resolve.ResolveTool, error)) *cobra.Command {
	var options repository.GCOptions
	command := &cobra.Command{
		Use:          "gc",
		Short:        "Pack retained objects and prune expired loose objects",
		Long:         "Retain branch, reflog, and durable merge-resolution roots; pack reachable objects; and prune only unreachable loose objects past the retention grace period. The complete JSON report is written when GC commits with a cleanup warning.",
		Example:      "  spl gc\n  spl gc --dry-run\n  spl gc --repack --grace-period 336h",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			tool, err := toolProvider()
			if err != nil {
				return err
			}
			result, gcErr := tool.EDGGC(command.Context(), options)
			if gcErr != nil {
				var warning *repository.GCCommittedWithWarningError
				if !errors.As(gcErr, &warning) {
					return gcErr
				}
			}
			if err := json.NewEncoder(command.OutOrStdout()).Encode(result); err != nil {
				return err
			}
			return gcErr
		},
	}
	command.Flags().BoolVar(&options.DryRun, "dry-run", false, "report maintenance work without changing object storage")
	command.Flags().BoolVar(&options.Repack, "repack", false, "compact active packs into one replacement generation")
	command.Flags().DurationVar(&options.GracePeriod, "grace-period", 0, "retain unreachable loose objects for this duration (default 336h)")
	return command
}
