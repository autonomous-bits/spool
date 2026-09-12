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
				"author": map[string]any{
					"type":        "string",
					"description": "Optional override commit author",
				},
				"message": map[string]any{
					"type":        "string",
					"description": "Optional override commit message",
				},
			},
			"required": []string{"branch"},
		},
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Branch  string `json:"branch"`
				DryRun  bool   `json:"dry_run,omitempty"`
				Force   bool   `json:"force,omitempty"`
				Author  string `json:"author,omitempty"`
				Message string `json:"message,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Branch == "" {
				return nil, errors.New("branch is required")
			}
			return withRepo(stateDirProvider, func(repo *repository.Repository) (any, error) {
				result, err := repo.Prune(repository.PruneRequest{
					Branch:  in.Branch,
					DryRun:  in.DryRun,
					Force:   in.Force,
					Author:  in.Author,
					Message: in.Message,
				})
				if err != nil {
					var warning *repository.PruneCommittedWithWarningError
					if errors.As(err, &warning) {
						return result, err
					}
					return nil, err
				}
				return result, nil
			})
		},
	}
}
