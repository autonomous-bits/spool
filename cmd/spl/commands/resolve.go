package commands

import (
	"fmt"

	"github.com/autonomous-bits/spool/internal/ctxgit"
	"github.com/spf13/cobra"
)

// NewResolveCommand creates the bound-only resolve command.
func NewResolveCommand(opts ctxgit.Options) *cobra.Command {
	var nodeID string
	command := &cobra.Command{
		Use:          "resolve",
		Short:        "Resolve a node from the bound context checkout",
		Long:         "Resolve a stable node ID from the bound context-git projection. Unbound workspaces are refused.",
		Example:      "  spl resolve --node idea-1",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			if nodeID == "" {
				return fmt.Errorf("node is required")
			}
			session, err := startBoundSession(command, opts)
			if err != nil {
				return err
			}
			result, err := session.ResolveResult(nodeID)
			if err != nil {
				return err
			}
			return writeJSON(command, result, "resolve")
		},
	}
	command.Flags().StringVar(&nodeID, "node", "", "stable node entity ID")
	_ = command.MarkFlagRequired("node")
	return command
}
