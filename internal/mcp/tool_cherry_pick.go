package mcp

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/autonomous-bits/spool/internal/repository"
)

func toolCherryPick(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_cherry_pick",
		Description: "Apply the exact graph delta of a historical commit onto a target branch without incorporating unrelated history.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"commit": map[string]any{
					"type":        "string",
					"description": "Source commit hash whose graph delta to transplant",
				},
				"target_branch": map[string]any{
					"type":        "string",
					"description": "Branch onto which to apply the transplanted delta",
				},
				"dry_run": map[string]any{
					"type":        "boolean",
					"description": "Simulate cherry-picking and report changes without modifying target branch",
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
			"required": []string{"commit", "target_branch"},
		},
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Commit       string `json:"commit"`
				TargetBranch string `json:"target_branch"`
				DryRun       bool   `json:"dry_run,omitempty"`
				Author       string `json:"author,omitempty"`
				Message      string `json:"message,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Commit == "" || in.TargetBranch == "" {
				return nil, errors.New("commit and target_branch are required")
			}
			return withRepo(stateDirProvider, func(repo *repository.Repository) (any, error) {
				result, err := repo.CherryPick(repository.CherryPickRequest{
					Commit:       in.Commit,
					TargetBranch: in.TargetBranch,
					DryRun:       in.DryRun,
					Author:       in.Author,
					Message:      in.Message,
				})
				if err != nil {
					var warning *repository.CherryPickCommittedWithWarningError
					if errors.As(err, &warning) || errors.Is(err, repository.ErrCherryPickConflicts) {
						return result, err
					}
					return nil, err
				}
				return result, nil
			})
		},
	}
}
