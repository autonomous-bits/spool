package mcp

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/autonomous-bits/spool/internal/repository"
)

func toolMigrate(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_migrate",
		Description: "Upgrade an existing Spool repository state directory to a newer format version.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"from": map[string]any{
					"type":        "integer",
					"description": "Source repository format version",
				},
				"to": map[string]any{
					"type":        "integer",
					"description": "Target repository format version",
				},
			},
			"required": []string{"from", "to"},
		},
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				From int `json:"from"`
				To   int `json:"to"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.From <= 0 || in.To <= 0 {
				return nil, errors.New("both 'from' and 'to' must be positive integers")
			}
			stateDir, err := stateDirProvider()
			if err != nil {
				return nil, err
			}
			return repository.MigrateRepositoryFormat(stateDir, in.From, in.To)
		},
	}
}
