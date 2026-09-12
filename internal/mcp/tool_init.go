package mcp

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/autonomous-bits/spool/internal/repository"
)

func toolInit(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_init",
		Description: "Initialize a new Spool repository in the resolved state directory.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Handler: func(_ context.Context, _ json.RawMessage) (any, error) {
			stateDir, err := stateDirProvider()
			if err != nil {
				return nil, err
			}
			repo, err := repository.InitializeRepository(stateDir)
			if err != nil {
				return nil, err
			}
			if closeErr := repo.Close(); closeErr != nil {
				return nil, fmt.Errorf("finalize repository initialization: %w", closeErr)
			}
			return map[string]string{"status": "initialized", "stateDir": stateDir}, nil
		},
	}
}
