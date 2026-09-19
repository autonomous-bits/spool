package commands

import (
	"github.com/autonomous-bits/spool/internal/ctxgit"
	"github.com/spf13/cobra"
)

func startBoundSession(command *cobra.Command, opts ctxgit.Options) (*ctxgit.Session, error) {
	return ctxgit.Start(command.Context(), opts)
}
