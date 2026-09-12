package mcp

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/autonomous-bits/spool/internal/repository"
)

func toolMergeApply(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_merge_apply",
		Description: "Apply a clean, reviewed merge preview to commit the merge.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"source": map[string]any{
					"type":        "string",
					"description": "Source branch to merge",
				},
				"target": map[string]any{
					"type":        "string",
					"description": "Target branch to update",
				},
				"transaction_id": map[string]any{
					"type":        "string",
					"description": "Transaction ID from merge preview",
				},
				"preview_id": map[string]any{
					"type":        "string",
					"description": "Preview commit ID from merge preview",
				},
				"author": map[string]any{
					"type":        "string",
					"description": "Optional author name",
				},
				"message": map[string]any{
					"type":        "string",
					"description": "Optional merge commit message",
				},
			},
			"required": []string{"source", "target", "transaction_id", "preview_id"},
		},
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Source        string `json:"source"`
				Target        string `json:"target"`
				TransactionID string `json:"transaction_id"`
				PreviewID     string `json:"preview_id"`
				Author        string `json:"author,omitempty"`
				Message       string `json:"message,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Source == "" || in.Target == "" || in.TransactionID == "" || in.PreviewID == "" {
				return nil, errors.New("source, target, transaction_id, and preview_id are required")
			}
			return withRepo(stateDirProvider, func(repo *repository.Repository) (any, error) {
				commit, err := repo.ApplyMergePreview(in.Source, in.Target, in.TransactionID, repository.ObjectID(in.PreviewID), in.Author, in.Message)
				if err != nil {
					return nil, err
				}
				return map[string]any{"commit": commit}, nil
			})
		},
	}
}
