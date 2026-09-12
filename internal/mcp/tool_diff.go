package mcp

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/autonomous-bits/spool/internal/resolve"
)

func toolDiff(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_diff",
		Description: "Compute structural graph diff (added, removed, modified nodes and edges) between base and target branches or explicit commits.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"base_branch": map[string]any{
					"type":        "string",
					"description": "Base branch name",
				},
				"base_commit": map[string]any{
					"type":        "string",
					"description": "Optional explicit reachable base commit",
				},
				"target_branch": map[string]any{
					"type":        "string",
					"description": "Target branch name",
				},
				"target_commit": map[string]any{
					"type":        "string",
					"description": "Optional explicit reachable target commit",
				},
			},
			"required": []string{"base_branch", "target_branch"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				BaseBranch   string  `json:"base_branch"`
				BaseCommit   *string `json:"base_commit,omitempty"`
				TargetBranch string  `json:"target_branch"`
				TargetCommit *string `json:"target_commit,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.BaseBranch == "" || in.TargetBranch == "" {
				return nil, errors.New("base_branch and target_branch are required")
			}
			return withTool(stateDirProvider, func(tool *resolve.ResolveTool) (any, error) {
				return tool.SPLDiff(ctx, resolve.DiffRequest{
					Base: resolve.SnapshotSelector{
						Branch: in.BaseBranch,
						Commit: in.BaseCommit,
					},
					Target: resolve.SnapshotSelector{
						Branch: in.TargetBranch,
						Commit: in.TargetCommit,
					},
				})
			})
		},
	}
}
