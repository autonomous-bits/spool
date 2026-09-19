package commands

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/autonomous-bits/spool/internal/ctxgit"
	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/spf13/cobra"
)

// NewMutateCommand writes one node/edge mutation batch via a short-lived branch + PR.
func NewMutateCommand(opts ctxgit.Options) *cobra.Command {
	var batchPath, author, message string
	command := &cobra.Command{
		Use:          "mutate",
		Short:        "Mutate the bound context graph",
		Long:         "Apply one JSON mutation-operation batch (nodes and edges) to the bound context checkout and open a short-lived branch + PR. Unbound workspaces are refused. Use schema migrate when changing schema.toml.",
		Example:      "  spl mutate --batch mutations.json --message \"Record requirement\"",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			data, err := os.ReadFile(batchPath)
			if err != nil {
				return fmt.Errorf("read mutation batch: %w", err)
			}
			var operations []repository.MutationOperation
			if err := json.Unmarshal(data, &operations); err != nil {
				return fmt.Errorf("decode mutation batch: %w", err)
			}
			session, err := startBoundSession(command, opts)
			if err != nil {
				return err
			}
			result, err := session.Mutate(command.Context(), ctxgit.MutateRequest{
				Operations: operations,
				Author:     author,
				Message:    message,
			})
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(result)
		},
	}
	command.Flags().StringVar(&batchPath, "batch", "", "path to a JSON mutation-operation array")
	command.Flags().StringVar(&author, "author", "", "git author")
	command.Flags().StringVar(&message, "message", "", "commit/PR message")
	_ = command.MarkFlagRequired("batch")
	return command
}
