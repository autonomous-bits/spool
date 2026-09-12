package mcp

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/autonomous-bits/spool/internal/resolve"
)

func toolSearch(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_search",
		Description: "Perform lexical full-text search (FTS5) across nodes in a branch head snapshot.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"branch": map[string]any{
					"type":        "string",
					"description": "Branch to search within",
				},
				"query": map[string]any{
					"type":        "string",
					"description": "Search query terms or keywords",
				},
				"continuation": map[string]any{
					"type":        "string",
					"description": "Optional pagination continuation token",
				},
			},
			"required": []string{"branch", "query"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Branch       string `json:"branch"`
				Query        string `json:"query"`
				Continuation string `json:"continuation,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Branch == "" {
				return nil, errors.New("branch is required")
			}
			if in.Query == "" {
				return nil, errors.New("query is required")
			}
			return withTool(stateDirProvider, func(tool *resolve.ResolveTool) (any, error) {
				return tool.SPLSearch(ctx, resolve.SearchRequest{
					Selector:          resolve.SnapshotSelector{Branch: in.Branch},
					Query:             in.Query,
					ContinuationToken: in.Continuation,
				})
			})
		},
	}
}
