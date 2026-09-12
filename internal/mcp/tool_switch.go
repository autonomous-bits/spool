package mcp

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/autonomous-bits/spool/internal/repository"
)

func toolSwitch(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_switch",
		Description: "Switch the repository's active branch.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Branch name to switch to",
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
				return repo.SwitchBranch(in.Name)
			})
		},
	}
}
