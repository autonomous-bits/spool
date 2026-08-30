package commands

import (
	"github.com/autonomous-bits/spool/internal/resolve"
	"github.com/spf13/cobra"
)

// NewSearchExpandCommand creates the spl search-expand command.
func NewSearchExpandCommand(toolProvider func() (*resolve.ResolveTool, error)) *cobra.Command {
	return newContextualCommand(
		"search-expand",
		"Search nodes and expand graph context",
		"Search lexical or typed-filter evidence, then return bounded graph context as JSON.",
		"spl search-expand --branch main --query incident --direction out --edge-type RELATES_TO",
		toolProvider,
		false,
	)
}
