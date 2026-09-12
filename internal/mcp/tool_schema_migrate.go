package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"github.com/autonomous-bits/spool/internal/repository"
)

func toolSchemaMigrate(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_schema_migrate",
		Description: "Stage a schema migration and conforming graph mutations atomically.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"branch": map[string]any{
					"type":        "string",
					"description": "Branch on which to stage the schema migration",
				},
				"schema_toml": map[string]any{
					"type":        "string",
					"description": "Target schema TOML content or path to a schema TOML file",
				},
				"operations": map[string]any{
					"type":        "array",
					"description": "Array of mutation operations conforming to the new schema",
					"items": map[string]any{
						"type": "object",
					},
				},
			},
			"required": []string{"branch", "schema_toml", "operations"},
		},
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Branch     string                         `json:"branch"`
				SchemaTOML string                         `json:"schema_toml"`
				Operations []repository.MutationOperation `json:"operations"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Branch == "" || in.SchemaTOML == "" {
				return nil, errors.New("branch and schema_toml are required")
			}

			schemaBytes := []byte(in.SchemaTOML)
			// Check if schema_toml is a file path
			if _, err := os.Stat(in.SchemaTOML); err == nil {
				data, readErr := os.ReadFile(in.SchemaTOML)
				if readErr == nil {
					schemaBytes = data
				}
			}

			return withRepo(stateDirProvider, func(repo *repository.Repository) (any, error) {
				return repo.StageSchemaMigration(repository.SchemaMigrationRequest{
					Branch:     in.Branch,
					SchemaTOML: schemaBytes,
					Operations: in.Operations,
				})
			})
		},
	}
}
