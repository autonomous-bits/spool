package commands

import (
	"encoding/json"

	"github.com/autonomous-bits/spool/internal/resolve"
	"github.com/spf13/cobra"
)

// NewValidateCommand creates the command that validates one immutable snapshot.
func NewValidateCommand(toolProvider func() (*resolve.ResolveTool, error)) *cobra.Command {
	var branchName, commit string
	command := &cobra.Command{
		Use:          "validate",
		Short:        "Validate a branch or reachable commit against its schema",
		Long:         "Pin a branch or select one reachable commit, then validate that immutable graph snapshot against its stored schema. The JSON report includes the selected snapshot and schema metadata.",
		Example:      "  spl validate --branch main\n  spl validate --branch main --commit <commit-id>",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			tool, err := toolProvider()
			if err != nil {
				return err
			}
			selector := resolve.SnapshotSelector{Branch: branchName}
			if command.Flags().Changed("commit") {
				selector.Commit = &commit
			}
			result, err := tool.SPLValidateSchema(command.Context(), resolve.SchemaValidationRequest{Selector: selector})
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(result)
		},
	}
	command.Flags().StringVar(&branchName, "branch", "", "branch to validate")
	command.Flags().StringVar(&commit, "commit", "", "reachable commit to validate")
	_ = command.MarkFlagRequired("branch")
	return command
}
