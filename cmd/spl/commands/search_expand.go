package commands

import (
	"github.com/autonomous-bits/spool/internal/contextual"
	"github.com/autonomous-bits/spool/internal/ctxgit"
	"github.com/spf13/cobra"
)

// NewSearchExpandCommand creates the bound-only search-expand command.
func NewSearchExpandCommand(opts ctxgit.Options) *cobra.Command {
	return newContextualCommand(
		"search-expand",
		"Search nodes and expand graph context",
		"Search lexical or typed-filter evidence on the bound checkout, then return bounded graph context as JSON.",
		"spl search-expand --query incident --direction out --edge-type RELATES_TO",
		opts,
		false,
	)
}

// NewQueryContextCommand creates the renamed bound-only query-context command.
func NewQueryContextCommand(opts ctxgit.Options) *cobra.Command {
	return newContextualCommand(
		"query-context",
		"Assemble evidence-focused graph context",
		"Return JSON evidence and bounded graph context from the bound checkout. The `context` namespace is only for init/export.",
		"spl query-context --label Task --property-text status=open --direction both",
		opts,
		true,
	)
}

func newContextualCommand(use, short, long, example string, opts ctxgit.Options, evidenceFirst bool) *cobra.Command {
	var query, direction string
	var edgeTypes []string
	var seedLimit int
	var filters retrievalFilterFlags
	command := &cobra.Command{
		Use: use, Short: short, Long: long, Example: "  " + example,
		Aliases: []string{}, Args: cobra.NoArgs, SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			predicates, err := filters.predicates()
			if err != nil {
				return err
			}
			if err := validateContextualSeedSelector(query, filters.labels, predicates); err != nil {
				return err
			}
			session, err := startBoundSession(command, opts)
			if err != nil {
				return err
			}
			dir := contextual.Direction(direction)
			if dir == "" {
				dir = contextual.DirectionOut
			}
			result, err := session.QueryContext(command.Context(), ctxgit.QueryContextRequest{
				Query:      query,
				Labels:     filters.labels,
				Predicates: predicates,
				Direction:  dir,
				EdgeTypes:  edgeTypes,
				SeedLimit:  seedLimit,
			})
			if err != nil {
				return err
			}
			name := "search-expand"
			if evidenceFirst {
				name = "query-context"
			}
			return writeJSON(command, result, name)
		},
	}
	command.Flags().StringVar(&query, "query", "", "lexical query (exclusive with filter flags)")
	command.Flags().StringVar(&direction, "direction", string(contextual.DirectionOut), "edge direction: out, in, or both")
	command.Flags().StringArrayVar(&edgeTypes, "edge-type", nil, "edge type to traverse (repeatable)")
	command.Flags().IntVar(&seedLimit, "seed-limit", 0, "maximum evidence seeds before expansion")
	filters.add(command)
	return command
}
