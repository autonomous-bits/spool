package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/autonomous-bits/spool/internal/repository"
)

func toolAdd(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_add",
		Description: "Validate and stage an array of graph-mutation operations on a branch directly in memory, without creating a file on disk.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"branch": map[string]any{
					"type":        "string",
					"description": "Branch on which to stage the mutation operations",
				},
				"operations": map[string]any{
					"type":        "array",
					"description": "Array of mutation operation objects (e.g. action: 'add', entity: 'node'|'edge', id, title, labels, properties)",
					"items": map[string]any{
						"type": "object",
					},
				},
			},
			"required": []string{"branch", "operations"},
		},
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Branch     string                         `json:"branch"`
				Operations []repository.MutationOperation `json:"operations"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, fmt.Errorf("decode operations: %w", err)
			}
			if in.Branch == "" {
				return nil, errors.New("branch is required")
			}
			if len(in.Operations) == 0 {
				return nil, errors.New("operations must not be empty")
			}
			return withRepo(stateDirProvider, func(repo *repository.Repository) (any, error) {
				return repo.StageMutationBatch(repository.StageMutationRequest{
					Branch:     in.Branch,
					Operations: in.Operations,
				})
			})
		},
	}
}
