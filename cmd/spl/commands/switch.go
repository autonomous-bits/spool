package commands

import (
	"encoding/json"

	"github.com/autonomous-bits/spool/internal/repository/branch"
	"github.com/autonomous-bits/spool/internal/resolve"
	"github.com/spf13/cobra"
)

// NewSwitchCommand creates the command for selecting an existing local branch.
func NewSwitchCommand(toolProvider func() (*resolve.ResolveTool, error)) *cobra.Command {
	return &cobra.Command{
		Use:          "switch <branch>",
		Short:        "Switch the active branch",
		Long:         "Make an existing local branch active and write the resulting active-branch state as JSON.",
		Example:      "  spl switch feature",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(command *cobra.Command, args []string) error {
			tool, err := toolProvider()
			if err != nil {
				return err
			}
			result, err := tool.SPLSwitchBranch(command.Context(), branch.SwitchRequest{Name: args[0]})
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(result)
		},
	}
}
