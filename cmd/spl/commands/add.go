package commands

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/resolve"
	"github.com/spf13/cobra"
)

// NewAddCommand creates the command for validating and staging a graph-mutation batch.
func NewAddCommand(toolProvider func() (*resolve.ResolveTool, error)) *cobra.Command {
	var branchName, batchPath string
	command := &cobra.Command{
		Use:   "add",
		Short: "Validate and stage a graph-mutation batch",
		Long: "Validate a JSON array of graph-mutation operations and stage it on a branch. " +
			"Node operations may include labels and typed properties; edge operations may include type and typed properties. " +
			"Each property is tagged with kind (null, bool, integer, float, string, list, or map), with lists and maps recursively containing tagged values. " +
			"The built-in v1 schema is permissive: it records these fields but does not enforce user-authored constraints. " +
			"The command writes the staged-set summary as JSON to standard output.",
		Example: "  spl add --branch main --batch mutations.json\n" +
			"  # mutations.json: [{\"action\":\"add\",\"entity\":\"node\",\"id\":\"requirement-1\",\"title\":\"Authenticate\",\"labels\":[\"Requirement\"],\"properties\":{\"priority\":{\"kind\":\"integer\",\"integer\":3}}}]",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			if err := command.Context().Err(); err != nil {
				return err
			}
			data, err := os.ReadFile(batchPath)
			if err != nil {
				return fmt.Errorf("read mutation batch: %w", err)
			}
			var operations []repository.MutationOperation
			if err := json.Unmarshal(data, &operations); err != nil {
				return fmt.Errorf("decode mutation batch: %w", err)
			}
			tool, err := toolProvider()
			if err != nil {
				return err
			}
			result, err := tool.SPLStageMutationBatch(command.Context(), repository.StageMutationRequest{
				Branch: branchName, Operations: operations,
			})
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(result)
		},
	}
	command.Flags().StringVar(&branchName, "branch", "", "branch on which to stage the batch")
	command.Flags().StringVar(&batchPath, "batch", "", "path to a JSON mutation-operation array")
	_ = command.MarkFlagRequired("branch")
	_ = command.MarkFlagRequired("batch")
	return command
}
