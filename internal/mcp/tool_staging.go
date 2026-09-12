package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

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

func toolAdd(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_add",
		Description: "Validate and stage an array of graph-mutation operations on a branch directly in memory, without creating a file on disk.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"branch": map[string]any{
					"type":        "string",
					"description": "Branch on which to stage the mutation operations",
				},
				"operations": map[string]any{
					"type":        "array",
					"description": "Array of mutation operation objects (e.g. action: 'add', entity: 'node'|'edge', id, title, labels, properties)",
					"items": map[string]any{
						"type": "object",
					},
				},
			},
			"required": []string{"branch", "operations"},
		},
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Branch     string                         `json:"branch"`
				Operations []repository.MutationOperation `json:"operations"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, fmt.Errorf("decode operations: %w", err)
			}
			if in.Branch == "" {
				return nil, errors.New("branch is required")
			}
			if len(in.Operations) == 0 {
				return nil, errors.New("operations must not be empty")
			}
			return withRepo(stateDirProvider, func(repo *repository.Repository) (any, error) {
				return repo.StageMutationBatch(repository.StageMutationRequest{
					Branch:     in.Branch,
					Operations: in.Operations,
				})
			})
		},
	}
}

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
						return result, err
					}
					return nil, err
				}
				return result, nil
			})
		},
	}
}
