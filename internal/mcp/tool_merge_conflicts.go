package mcp

import (
	"context"
	"encoding/json"

	"github.com/autonomous-bits/spool/internal/repository"
)

func toolMergeConflicts(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_merge_conflicts",
		Description: "Inspect conflicts recorded for an active merge transaction.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"target": map[string]any{
					"type":        "string",
					"description": "Target branch of the merge",
				},
				"transaction_id": map[string]any{
					"type":        "string",
					"description": "Transaction ID",
				},
			},
			"required": []string{"target", "transaction_id"},
		},
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Target        string `json:"target"`
				TransactionID string `json:"transaction_id"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			return withRepo(stateDirProvider, func(repo *repository.Repository) (any, error) {
				return repo.InspectMergeTransaction(in.Target, in.TransactionID)
			})
		},
	}
}
