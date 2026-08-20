package commands

import (
	"time"

	"github.com/autonomous-bits/spool/internal/resolve"
	"github.com/spf13/cobra"
)

type queryBudgetFlags struct {
	maxRows          int
	maxResponseBytes int
	maxDepth         int
	maxVisited       int
	timeout          time.Duration
}

func (flags *queryBudgetFlags) addReadBudgetFlags(command *cobra.Command) {
	command.Flags().IntVar(&flags.maxRows, "max-rows", 0, "maximum rows to return")
	command.Flags().IntVar(&flags.maxResponseBytes, "max-response-bytes", 0, "maximum response size in bytes")
	command.Flags().DurationVar(&flags.timeout, "timeout", 0, "maximum query duration")
}

func (flags *queryBudgetFlags) addTraversalBudgetFlags(command *cobra.Command) {
	command.Flags().IntVar(&flags.maxDepth, "max-depth", 0, "maximum traversal depth")
	command.Flags().IntVar(&flags.maxVisited, "max-visited", 0, "maximum visited nodes")
}

func (flags *queryBudgetFlags) addPagedQueryFlags(command *cobra.Command, continuation *string) {
	flags.addReadBudgetFlags(command)
	command.Flags().StringVar(continuation, "continuation", "", "continuation token")
}

func (flags *queryBudgetFlags) request(command *cobra.Command) resolve.QueryBudgetRequest {
	budget := resolve.QueryBudgetRequest{}
	if command.Flags().Changed("max-rows") {
		budget.MaxRows = &flags.maxRows
	}
	if command.Flags().Changed("max-response-bytes") {
		budget.MaxResponseBytes = &flags.maxResponseBytes
	}
	if command.Flags().Changed("max-depth") {
		budget.MaxDepth = &flags.maxDepth
	}
	if command.Flags().Changed("max-visited") {
		budget.MaxVisited = &flags.maxVisited
	}
	if command.Flags().Changed("timeout") {
		budget.Timeout = &flags.timeout
	}
	return budget
}
