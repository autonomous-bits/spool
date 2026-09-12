package mcp

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/autonomous-bits/spool/internal/resolve"
)

func toolFilter(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_filter",
		Description: "Filter nodes in a branch head snapshot by labels and optional property predicates.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"branch": map[string]any{
					"type":        "string",
					"description": "Branch to filter",
				},
				"labels": map[string]any{
					"type":        "array",
					"description": "Node labels to match (e.g. ['Requirement', 'Decision'])",
					"items": map[string]any{
						"type": "string",
					},
				},
				"continuation": map[string]any{
					"type":        "string",
					"description": "Optional pagination continuation token",
				},
			},
			"required": []string{"branch"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Branch       string   `json:"branch"`
				Labels       []string `json:"labels,omitempty"`
				Continuation string   `json:"continuation,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Branch == "" {
				return nil, errors.New("branch is required")
			}
			return withTool(stateDirProvider, func(tool *resolve.ResolveTool) (any, error) {
				return tool.SPLFilter(ctx, resolve.FilterRequest{
					Selector:          resolve.SnapshotSelector{Branch: in.Branch},
					Labels:            in.Labels,
					ContinuationToken: in.Continuation,
				})
			})
		},
	}
}
