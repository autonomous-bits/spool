package mcp

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/autonomous-bits/spool/internal/repository"
)

func toolMergePreview(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_merge_preview",
		Description: "Compute a deterministic three-way merge preview between source and target branches.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"source": map[string]any{
					"type":        "string",
					"description": "Source branch to merge",
				},
				"target": map[string]any{
					"type":        "string",
					"description": "Target branch to update",
				},
			},
			"required": []string{"source", "target"},
		},
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Source string `json:"source"`
				Target string `json:"target"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Source == "" || in.Target == "" {
				return nil, errors.New("source and target are required")
			}
			return withRepo(stateDirProvider, func(repo *repository.Repository) (any, error) {
				return repo.PreviewMerge(in.Source, in.Target)
			})
		},
	}
}
