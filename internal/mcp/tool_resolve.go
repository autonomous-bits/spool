package mcp

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/autonomous-bits/spool/internal/resolve"
)

func toolResolve(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_resolve",
		Description: "Resolve an immutable node entity by stable ID from a branch snapshot or explicit commit.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"branch": map[string]any{
					"type":        "string",
					"description": "Branch to resolve the node from",
				},
				"node": map[string]any{
					"type":        "string",
					"description": "Stable node entity ID",
				},
				"commit": map[string]any{
					"type":        "string",
					"description": "Optional reachable commit ID to resolve against",
				},
			},
			"required": []string{"branch", "node"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Branch string  `json:"branch"`
				Node   string  `json:"node"`
				Commit *string `json:"commit,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Branch == "" {
				return nil, errors.New("branch is required")
			}
			if in.Node == "" {
				return nil, errors.New("node is required")
			}
			return withTool(stateDirProvider, func(tool *resolve.ResolveTool) (any, error) {
				return tool.SPLResolve(ctx, resolve.ResolveRequest{
					Selector: resolve.SnapshotSelector{
						Branch: in.Branch,
						Commit: in.Commit,
					},
					NodeID: in.Node,
				})
			})
		},
	}
}
