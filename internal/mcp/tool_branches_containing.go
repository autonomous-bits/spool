package mcp

import (
	"context"
	"encoding/json"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/resolve"
)

func toolBranchesContaining(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_branches_containing",
		Description: "Find all repository branches containing a specific entity or snapshot ID.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"entity_id": map[string]any{
					"type":        "string",
					"description": "Stable entity or node ID to find",
				},
				"snapshot_id": map[string]any{
					"type":        "string",
					"description": "Exact graph snapshot object ID to find",
				},
				"continuation": map[string]any{
					"type":        "string",
					"description": "Optional pagination continuation token",
				},
			},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				EntityID     string `json:"entity_id,omitempty"`
				SnapshotID   string `json:"snapshot_id,omitempty"`
				Continuation string `json:"continuation,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			return withTool(stateDirProvider, func(tool *resolve.ResolveTool) (any, error) {
				return tool.SPLBranchesContainingPage(ctx, resolve.BranchesContainingRequest{
					Selector: resolve.ContainmentSelector{
						EntityID:   in.EntityID,
						SnapshotID: repository.ObjectID(in.SnapshotID),
					},
					ContinuationToken: in.Continuation,
				})
			})
		},
	}
}
