package mcp

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/autonomous-bits/spool/internal/repository"
)

func toolBranchDelete(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_branch_delete",
		Description: "Delete an inactive, non-default local branch.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Branch name to delete",
				},
			},
			"required": []string{"name"},
		},
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Name == "" {
				return nil, errors.New("name is required")
			}
			return withRepo(stateDirProvider, func(repo *repository.Repository) (any, error) {
				return repo.DeleteBranch(in.Name)
			})
		},
	}
}
