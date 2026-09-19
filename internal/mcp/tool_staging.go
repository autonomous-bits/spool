package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/autonomous-bits/spool/internal/ctxgit"
	"github.com/autonomous-bits/spool/internal/repository"
)

func toolStatus(rt *runtime) Tool {
	return Tool{
		Name:        "spl_status",
		Description: "Report staged mutation delta as JSON. Bound workspaces report the context-git batch. Unbound leftover `.spl` staging is deprecated and unsupported for solution context (migration-only).",
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
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Branch string `json:"branch"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Branch == "" {
				return nil, errors.New("branch is required")
			}
			session, err := rt.requireSession(ctx)
			if err == nil {
				return session.Status()
			}
			if errors.Is(err, ctxgit.ErrUnbound) {
				return withRepo(rt.stateDir, func(repo *repository.Repository) (any, error) {
					return repo.BranchStagingStatus(in.Branch)
				})
			}
			return nil, err
		},
	}
}

func toolAdd(rt *runtime) Tool {
	return Tool{
		Name:        "spl_add",
		Description: "Validate a graph-mutation batch. When the workspace is bound, the batch is staged for one context-git commit and PR. Unbound workspaces refuse writes.",
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
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
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
			session, err := rt.requireSession(ctx)
			if err != nil {
				return nil, err
			}
			return session.Stage(in.Operations)
		},
	}
}

func toolCommit(rt *runtime) Tool {
	return Tool{
		Name:        "spl_commit",
		Description: "Commit the staged mutation batch to the bound context git remote on a short-lived branch and open a PR to the protected branch.",
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
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
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
			session, err := rt.requireSession(ctx)
			if err != nil {
				return nil, err
			}
			return session.Commit(ctx, in.Author, in.Message)
		},
	}
}
