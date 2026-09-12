package mcp

import (
	"context"
	"encoding/json"

	"github.com/autonomous-bits/spool/internal/repository"
)

func toolRemoteRemove(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_remote_remove",
		Description: "Remove the repository's configured Rack remote.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Handler: func(_ context.Context, _ json.RawMessage) (any, error) {
			return withRepo(stateDirProvider, func(repo *repository.Repository) (any, error) {
				_, existed, err := repo.Remote()
				if err != nil {
					return nil, err
				}
				if err := repo.RemoveRemote(); err != nil {
					return nil, err
				}
				return map[string]bool{"removed": existed}, nil
			})
		},
	}
}
