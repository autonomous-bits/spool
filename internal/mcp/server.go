package mcp

import (
	"context"

	"github.com/autonomous-bits/spool/assets"
	"github.com/autonomous-bits/spool/internal/version"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// NewSpoolServer creates and configures an MCP server with all Spool tools registered.
func NewSpoolServer(stateDirProvider func() (string, error)) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:        "spool",
		Title:       "Spool",
		Description: "A graph-based version control system for autonomous agent workflows",
		Version:     version.Version,
		Icons: []mcp.Icon{
			{
				Source:   assets.IconPNGDataURI,
				MIMEType: "image/png",
				Sizes:    []string{"192x192"},
			},
		},
	}, nil)

	RegisterAllTools(server, stateDirProvider)
	return server
}

// RunStdio executes the MCP server over standard input and standard output.
func RunStdio(ctx context.Context, server *mcp.Server) error {
	return server.Run(ctx, &mcp.StdioTransport{})
}
