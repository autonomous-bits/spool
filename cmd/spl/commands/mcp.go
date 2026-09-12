package commands

import (
	"github.com/autonomous-bits/spool/internal/mcp"
	"github.com/autonomous-bits/spool/internal/version"
	"github.com/spf13/cobra"
)

// NewMCPCommand creates the mcp command for running an MCP server over stdio.
func NewMCPCommand(stateDirProvider func() (string, error)) *cobra.Command {
	command := &cobra.Command{
		Use:          "mcp",
		Short:        "Start an MCP (Model Context Protocol) server over standard I/O",
		Long:         "Start a standard MCP JSON-RPC 2.0 server over stdin/stdout, allowing AI agents to interact with the Spool repository via structured, typed tools.",
		Example:      "  spl mcp\n  spl mcp --state-dir /path/to/.spl",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			server := mcp.NewServer("spool", version.Get().Version)
			server.RegisterTools(mcp.NewSpoolTools(stateDirProvider))
			return server.Serve(command.Context(), command.InOrStdin(), command.OutOrStdout())
		},
	}
	return command
}
