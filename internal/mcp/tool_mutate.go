package mcp

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/autonomous-bits/spool/internal/ctxgit"
	"github.com/autonomous-bits/spool/internal/repository"
)

func toolMutate(rt *runtime) Tool {
	return Tool{
		Name:        "spl_mutate",
		Description: "Apply one node/edge mutation-operation batch to the bound context graph via a short-lived branch and PR. Bound-only. Use schema migrate when changing schema.toml.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"operations": map[string]any{
					"type":        "array",
					"description": "Mutation operations (add/update/delete of nodes and edges)",
					"items":       map[string]any{"type": "object"},
				},
				"author":  map[string]any{"type": "string", "description": "Optional git author"},
				"message": map[string]any{"type": "string", "description": "Optional commit/PR message"},
			},
			"required": []string{"operations"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Operations []repository.MutationOperation `json:"operations"`
				Author     string                         `json:"author"`
				Message    string                         `json:"message"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if len(in.Operations) == 0 {
				return nil, errors.New("operations is required")
			}
			session, err := rt.requireSession(ctx)
			if err != nil {
				return nil, err
			}
			return session.Mutate(ctx, ctxgit.MutateRequest{
				Operations: in.Operations,
				Author:     in.Author,
				Message:    in.Message,
			})
		},
	}
}
