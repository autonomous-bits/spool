package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/autonomous-bits/spool/internal/ctxgit"
	"github.com/autonomous-bits/spool/internal/repository"
)

func toolPrune(rt *runtime) Tool {
	return Tool{
		Name:        "spl_prune",
		Description: "Prune Ephemeral nodes and cascading edges from the bound context checkout via a short-lived branch and PR.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"dry_run": map[string]any{"type": "boolean", "description": "Simulate pruning without writing a branch or PR"},
				"author":  map[string]any{"type": "string", "description": "Optional git author"},
				"message": map[string]any{"type": "string", "description": "Optional commit/PR message"},
			},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				DryRun  bool   `json:"dry_run,omitempty"`
				Author  string `json:"author,omitempty"`
				Message string `json:"message,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			session, err := rt.requireSession(ctx)
			if err != nil {
				return nil, err
			}
			return session.Prune(ctx, ctxgit.PruneRequest{DryRun: in.DryRun, Author: in.Author, Message: in.Message})
		},
	}
}

func toolSchemaMigrate(rt *runtime) Tool {
	return Tool{
		Name:        "spl_schema_migrate",
		Description: "Write a schema.toml migration and conforming graph mutations on a short-lived branch + PR.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"schema_toml": map[string]any{"type": "string", "description": "Inline TOML schema content or path to a schema.toml file"},
				"operations":  map[string]any{"type": "array", "description": "Optional conforming mutation operations", "items": map[string]any{"type": "object"}},
				"author":      map[string]any{"type": "string"},
				"message":     map[string]any{"type": "string"},
			},
			"required": []string{"schema_toml"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				SchemaTOML string                         `json:"schema_toml"`
				Operations []repository.MutationOperation `json:"operations"`
				Author     string                         `json:"author"`
				Message    string                         `json:"message"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.SchemaTOML == "" {
				return nil, errors.New("schema_toml is required")
			}
			schemaBytes := []byte(in.SchemaTOML)
			if !strings.Contains(in.SchemaTOML, "\n") && len(in.SchemaTOML) < 1024 {
				if fi, err := os.Stat(in.SchemaTOML); err == nil && !fi.IsDir() {
					data, readErr := os.ReadFile(in.SchemaTOML)
					if readErr != nil {
						return nil, fmt.Errorf("read schema file: %w", readErr)
					}
					schemaBytes = data
				}
			}
			session, err := rt.requireSession(ctx)
			if err != nil {
				return nil, err
			}
			return session.MigrateSchema(ctx, ctxgit.SchemaMigrateRequest{
				SchemaTOML: schemaBytes, Operations: in.Operations, Author: in.Author, Message: in.Message,
			})
		},
	}
}

func toolValidate(rt *runtime) Tool {
	return Tool{
		Name:        "spl_validate",
		Description: "Validate the bound context-git checkout against schema.toml.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			session, err := rt.requireSession(ctx)
			if err != nil {
				return nil, err
			}
			return session.ValidateSchema()
		},
	}
}
