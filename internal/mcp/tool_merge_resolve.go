package mcp

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/autonomous-bits/spool/internal/repository"
)

func toolMergeResolve(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_merge_resolve",
		Description: "Resolve every conflict in a persisted merge transaction using resolution selections and optional mutation overrides.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"target": map[string]any{
					"type":        "string",
					"description": "Target branch with the conflicted merge",
				},
				"transaction_id": map[string]any{
					"type":        "string",
					"description": "Owning merge transaction identifier",
				},
				"preview_id": map[string]any{
					"type":        "string",
					"description": "Persisted preview identifier",
				},
				"selections": map[string]any{
					"type":        "array",
					"description": "Array of merge resolution selection objects (entity: 'node'|'edge', id, resolution: 'source'|'target'|'delete'|'custom')",
					"items": map[string]any{
						"type": "object",
					},
				},
				"overrides": map[string]any{
					"type":        "array",
					"description": "Optional array of mutation operations providing custom resolutions",
					"items": map[string]any{
						"type": "object",
					},
				},
			},
			"required": []string{"target", "transaction_id", "preview_id", "selections"},
		},
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Target        string                                `json:"target"`
				TransactionID string                                `json:"transaction_id"`
				PreviewID     string                                `json:"preview_id"`
				Selections    []repository.MergeResolutionSelection `json:"selections"`
				Overrides     []repository.MutationOperation        `json:"overrides,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Target == "" || in.TransactionID == "" || in.PreviewID == "" {
				return nil, errors.New("target, transaction_id, and preview_id are required")
			}
			return withRepo(stateDirProvider, func(repo *repository.Repository) (any, error) {
				if err := repo.ResolveConflictedMerge(repository.ResolveConflictedMergeRequest{
					TargetBranch:  in.Target,
					TransactionID: in.TransactionID,
					PreviewID:     repository.ObjectID(in.PreviewID),
					Selections:    in.Selections,
					Overrides:     in.Overrides,
				}); err != nil {
					return nil, err
				}
				return map[string]any{"resolved": true}, nil
			})
		},
	}
}
