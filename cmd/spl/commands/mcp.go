package commands

import (
	"errors"
	"io"
	"strings"

	"github.com/autonomous-bits/spool/internal/mcp"
	officialmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

// NewMCPCommand creates the mcp command for running an MCP server over stdio.
func NewMCPCommand(stateDirProvider func() (string, error)) *cobra.Command {
	command := &cobra.Command{
		Use:          "mcp",
		Short:        "Start an MCP (Model Context Protocol) server over standard I/O",
		Long:         "Start an official MCP server over stdin/stdout, allowing AI agents to interact with the Spool repository via structured, typed tools.",
		Example:      "  spl mcp\n  spl mcp --state-dir /path/to/.spl",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			server := mcp.NewSpoolServer(stateDirProvider)
			in := command.InOrStdin()
			out := command.OutOrStdout()
			var r io.ReadCloser
			if rc, ok := in.(io.ReadCloser); ok {
				r = rc
			} else {
				r = io.NopCloser(in)
			}
			var w io.WriteCloser
			if wc, ok := out.(io.WriteCloser); ok {
				w = wc
			} else {
				w = nopWriteCloser{Writer: out}
			}
			err := server.Run(command.Context(), &officialmcp.IOTransport{
				Reader: r,
				Writer: w,
			})
			if err != nil {
				println("DEBUG ERR:", err.Error())
			}
			if err != nil && (errors.Is(err, io.EOF) || strings.Contains(err.Error(), "EOF") || strings.Contains(err.Error(), "server is closing") || command.Context().Err() != nil) {
				return nil
			}
			return err
		},
	}
	return command
}

type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error { return nil }
