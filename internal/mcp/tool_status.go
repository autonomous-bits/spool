package mcp

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/autonomous-bits/spool/internal/repository"
)

func toolStatus(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_status",
		Description: "Report a branch's staged mutation delta as JSON. A branch with no staged changes returns an empty delta.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"branch": map[string]any{
					"type":        "string",
					"description": "Branch whose staged delta to report",
				},
			},
			"required": []string{"branch"},
		},
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Branch string `json:"branch"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Branch == "" {
				return nil, errors.New("branch is required")
			}
			return withRepo(stateDirProvider, func(repo *repository.Repository) (any, error) {
				return repo.BranchStagingStatus(in.Branch)
			})
		},
	}
}
