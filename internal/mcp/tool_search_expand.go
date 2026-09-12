package mcp

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/autonomous-bits/spool/internal/contextual"
	"github.com/autonomous-bits/spool/internal/resolve"
)

func toolSearchExpand(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_search_expand",
		Description: "Select lexical or typed-filter seed evidence, then expand bounded graph context from those seeds.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"branch": map[string]any{
					"type":        "string",
					"description": "Branch to query",
				},
				"query": map[string]any{
					"type":        "string",
					"description": "Lexical search query for seed matching (mutually exclusive with label)",
				},
				"label": map[string]any{
					"type":        "string",
					"description": "Label filter for seed matching (mutually exclusive with query)",
				},
				"direction": map[string]any{
					"type":        "string",
					"description": "Edge traversal direction: 'out', 'in', or 'both' (default 'out')",
					"enum":        []string{"out", "in", "both"},
				},
				"edge_types": map[string]any{
					"type":        "array",
					"description": "Optional edge types to traverse",
					"items": map[string]any{
						"type": "string",
					},
				},
				"seed_limit": map[string]any{
					"type":        "integer",
					"description": "Maximum number of seed nodes to expand from",
				},
			},
			"required": []string{"branch"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Branch    string   `json:"branch"`
				Query     string   `json:"query,omitempty"`
				Label     string   `json:"label,omitempty"`
				Direction string   `json:"direction,omitempty"`
				EdgeTypes []string `json:"edge_types,omitempty"`
				SeedLimit int      `json:"seed_limit,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Branch == "" {
				return nil, errors.New("branch is required")
			}
			if in.Query != "" && in.Label != "" {
				return nil, errors.New("query and label are mutually exclusive")
			}
			if in.Query == "" && in.Label == "" {
				return nil, errors.New("either query or label must be specified")
			}

			dir := contextual.DirectionOut
			switch in.Direction {
			case "in":
				dir = contextual.DirectionIn
			case "both":
				dir = contextual.DirectionBoth
			}

			seeds := contextual.SeedSelector{}
			if in.Query != "" {
				seeds.Query = in.Query
			} else {
				seeds.Labels = []string{in.Label}
			}

			return withTool(stateDirProvider, func(tool *resolve.ResolveTool) (any, error) {
				return tool.SPLSearchExpand(ctx, resolve.SearchExpandRequest{
					Selector:  resolve.SnapshotSelector{Branch: in.Branch},
					Seeds:     seeds,
					Direction: dir,
					EdgeTypes: in.EdgeTypes,
					SeedLimit: in.SeedLimit,
				})
			})
		},
	}
}
