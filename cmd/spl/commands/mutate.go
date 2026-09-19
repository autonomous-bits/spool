package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/autonomous-bits/spool/internal/ctxgit"
	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/spf13/cobra"
)

// NewMutateCommand writes one node/edge mutation batch via a short-lived branch + PR.
func NewMutateCommand(opts ctxgit.Options) *cobra.Command {
	var operationsPath, author, message string
	command := &cobra.Command{
		Use:          "mutate",
		Short:        "Mutate the bound context graph",
		Long:         "Apply one JSON mutation-operation batch (nodes and edges) to the bound context checkout and open a short-lived branch + PR. Unbound workspaces are refused. Identical or empty diffs do not open a PR. Use schema migrate when changing schema.toml.",
		Example:      "  spl mutate --operations mutations.json --message \"Record requirement\"\n  cat mutations.json | spl mutate --operations - --message \"Record requirement\"",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			operations, err := readMutationOperations(command, operationsPath)
			if err != nil {
				return err
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
	command.Flags().StringVar(&operationsPath, "operations", "", "JSON mutation-operation array (file path, or - for stdin)")
	command.Flags().StringVar(&author, "author", "", "git author")
	command.Flags().StringVar(&message, "message", "", "commit/PR message")
	_ = command.MarkFlagRequired("operations")
	return command
}

func readMutationOperations(command *cobra.Command, path string) ([]repository.MutationOperation, error) {
	path = strings.TrimSpace(path)
	var data []byte
	var err error
	if path == "-" {
		data, err = io.ReadAll(command.InOrStdin())
		if err != nil {
			return nil, fmt.Errorf("read mutation operations from stdin: %w", err)
		}
	} else {
		data, err = os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read mutation operations: %w", err)
		}
	}
	var operations []repository.MutationOperation
	if err := json.Unmarshal(data, &operations); err != nil {
		return nil, fmt.Errorf("decode mutation operations: %w", err)
	}
	return operations, nil
}
