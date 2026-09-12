package mcp

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/autonomous-bits/spool/internal/resolve"
)

func toolGraph(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_graph",
		Description: "Export every node and edge in an immutable branch snapshot as JSON.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"branch": map[string]any{
					"type":        "string",
					"description": "Branch to export",
				},
				"commit": map[string]any{
					"type":        "string",
					"description": "Optional reachable commit ID",
				},
			},
			"required": []string{"branch"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Branch string  `json:"branch"`
				Commit *string `json:"commit,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Branch == "" {
				return nil, errors.New("branch is required")
			}
			return withTool(stateDirProvider, func(tool *resolve.ResolveTool) (any, error) {
				return tool.SPLGraph(ctx, resolve.SnapshotSelector{
					Branch: in.Branch,
					Commit: in.Commit,
				})
			})
		},
	}
}
