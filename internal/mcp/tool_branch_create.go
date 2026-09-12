package mcp

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/autonomous-bits/spool/internal/repository"
)

func toolBranchCreate(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_branch_create",
		Description: "Create a new branch from an existing source branch or commit.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Name of the new branch to create",
				},
				"from_branch": map[string]any{
					"type":        "string",
					"description": "Existing branch to use as source",
				},
				"from_commit": map[string]any{
					"type":        "string",
					"description": "Existing commit ID to use as source",
				},
			},
			"required": []string{"name"},
		},
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Name       string `json:"name"`
				FromBranch string `json:"from_branch,omitempty"`
				FromCommit string `json:"from_commit,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Name == "" {
				return nil, errors.New("name is required")
			}
			return withRepo(stateDirProvider, func(repo *repository.Repository) (any, error) {
				return repo.CreateBranch(in.Name, repository.BranchSource{
					Branch: in.FromBranch,
					Commit: in.FromCommit,
				})
			})
		},
	}
}
