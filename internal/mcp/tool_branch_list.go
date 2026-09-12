package mcp

import (
	"context"
	"encoding/json"

	"github.com/autonomous-bits/spool/internal/repository"
)

func toolBranchList(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_branch_list",
		Description: "List all local branches in the Spool repository.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Handler: func(_ context.Context, _ json.RawMessage) (any, error) {
			return withRepo(stateDirProvider, func(repo *repository.Repository) (any, error) {
				return repo.ListBranches()
			})
		},
	}
}
