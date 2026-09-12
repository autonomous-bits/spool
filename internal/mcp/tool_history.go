package mcp

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/autonomous-bits/spool/internal/resolve"
)

func toolHistory(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_history",
		Description: "Retrieve commit history and changes for a specific node or entity ID within a branch.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"branch": map[string]any{
					"type":        "string",
					"description": "Branch to query",
				},
				"node": map[string]any{
					"type":        "string",
					"description": "Entity or node ID",
				},
			},
			"required": []string{"branch", "node"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Branch string `json:"branch"`
				Node   string `json:"node"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Branch == "" || in.Node == "" {
				return nil, errors.New("branch and node are required")
			}
			return withTool(stateDirProvider, func(tool *resolve.ResolveTool) (any, error) {
				return tool.SPLHistory(ctx, resolve.HistoryRequest{
					Selector: resolve.SnapshotSelector{Branch: in.Branch},
					EntityID: in.Node,
				})
			})
		},
	}
}
