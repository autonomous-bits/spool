package mcp

import (
	"context"
	"encoding/json"

	"github.com/autonomous-bits/spool/internal/repository"
)

func toolFsck(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_fsck",
		Description: "Check repository object store, DAG integrity, and durability invariants.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Handler: func(ctx context.Context, _ json.RawMessage) (any, error) {
			stateDir, err := stateDirProvider()
			if err != nil {
				return nil, err
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return repository.FsckRepository(stateDir)
		},
	}
}
