package mcp

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/autonomous-bits/spool/internal/repository"
)

func toolPrune(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_prune",
		Description: "Prune temporary planning entities (nodes labeled Ephemeral) and cascading incident edges from a branch.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"branch": map[string]any{
					"type":        "string",
					"description": "Branch to prune",
				},
				"dry_run": map[string]any{
					"type":        "boolean",
					"description": "Simulate pruning without committing changes",
				},
				"force": map[string]any{
					"type":        "boolean",
					"description": "Allow pruning on default protected branch",
				},
			},
			"required": []string{"branch"},
		},
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Branch string `json:"branch"`
				DryRun bool   `json:"dry_run,omitempty"`
				Force  bool   `json:"force,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Branch == "" {
				return nil, errors.New("branch is required")
			}
			return withRepo(stateDirProvider, func(repo *repository.Repository) (any, error) {
				return repo.Prune(repository.PruneRequest{
					Branch: in.Branch,
					DryRun: in.DryRun,
					Force:  in.Force,
				})
			})
		},
	}
}
