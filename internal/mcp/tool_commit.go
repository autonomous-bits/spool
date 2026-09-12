package mcp

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/autonomous-bits/spool/internal/repository"
)

func toolCommit(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_commit",
		Description: "Commit all staged graph mutations for a branch into an immutable commit object.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"branch": map[string]any{
					"type":        "string",
					"description": "Branch whose staged mutations to commit",
				},
				"author": map[string]any{
					"type":        "string",
					"description": "Commit author (e.g. agent name or username)",
				},
				"message": map[string]any{
					"type":        "string",
					"description": "Commit message describing the graph changes",
				},
			},
			"required": []string{"branch", "message"},
		},
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Branch  string `json:"branch"`
				Author  string `json:"author"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Branch == "" {
				return nil, errors.New("branch is required")
			}
			if in.Message == "" {
				return nil, errors.New("message is required")
			}
			return withRepo(stateDirProvider, func(repo *repository.Repository) (any, error) {
				result, err := repo.CommitStagedMutationBatch(repository.CommitStagedMutationRequest{
					Branch:  in.Branch,
					Author:  in.Author,
					Message: in.Message,
				})
				if err != nil {
					var warning *repository.CommittedWithWarningError
					if errors.As(err, &warning) {
						return result, nil
					}
					return nil, err
				}
				return result, nil
			})
		},
	}
}
