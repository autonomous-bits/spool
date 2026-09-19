package commands

import (
	"errors"
	"io"
	"os"
	"strings"

	"github.com/autonomous-bits/spool/internal/ctxgit"
	"github.com/autonomous-bits/spool/internal/mcp"
	officialmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

// NewMCPCommand creates the mcp command for running an MCP server over stdio.
func NewMCPCommand(opts ctxgit.Options) *cobra.Command {
	command := &cobra.Command{
		Use:          "mcp",
		Short:        "Start an MCP (Model Context Protocol) server over standard I/O",
		Long:         "Start an official MCP server over stdin/stdout exposing the bound context-git KEEP tool set.",
		Example:      "  spl mcp",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			server := mcp.NewSpoolServerWithOptions(mcp.ServerOptions{
				WorkspaceDir: func() (string, error) {
					if opts.WorkspaceDir != "" {
						return opts.WorkspaceDir, nil
					}
					return os.Getwd()
				},
				CacheDir: opts.CacheDir,
				PROpener: opts.PROpener,
				Git:      opts.Git,
			})
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
			if err != nil && (errors.Is(err, io.EOF) || strings.Contains(err.Error(), "server is closing") || command.Context().Err() != nil) {
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
