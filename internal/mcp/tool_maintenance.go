package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/resolve"
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

func toolPrune(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_prune",
		Description: "Prune temporary planning entities (nodes labeled Ephemeral) and cascading incident edges from a branch.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"branch": map[string]any{
					"type":        "string",
					"description": "Branch to prune",
				},
				"dry_run": map[string]any{
					"type":        "boolean",
					"description": "Simulate pruning without committing changes",
				},
				"force": map[string]any{
					"type":        "boolean",
					"description": "Allow pruning on default protected branch",
				},
				"author": map[string]any{
					"type":        "string",
					"description": "Optional override commit author",
				},
				"message": map[string]any{
					"type":        "string",
					"description": "Optional override commit message",
				},
			},
			"required": []string{"branch"},
		},
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Branch  string `json:"branch"`
				DryRun  bool   `json:"dry_run,omitempty"`
				Force   bool   `json:"force,omitempty"`
				Author  string `json:"author,omitempty"`
				Message string `json:"message,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Branch == "" {
				return nil, errors.New("branch is required")
			}
			return withRepo(stateDirProvider, func(repo *repository.Repository) (any, error) {
				result, err := repo.Prune(repository.PruneRequest{
					Branch:  in.Branch,
					DryRun:  in.DryRun,
					Force:   in.Force,
					Author:  in.Author,
					Message: in.Message,
				})
				if err != nil {
					var warning *repository.PruneCommittedWithWarningError
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

func toolGC(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_gc",
		Description: "Pack reachable objects into packfiles and prune unreachable loose objects past the retention grace period.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"dry_run": map[string]any{
					"type":        "boolean",
					"description": "Report maintenance work without modifying object storage",
				},
				"repack": map[string]any{
					"type":        "boolean",
					"description": "Compact active packs into one replacement generation",
				},
				"grace_period": map[string]any{
					"type":        "string",
					"description": "Duration string to retain unreachable loose objects (e.g. '336h')",
				},
			},
		},
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				DryRun      bool   `json:"dry_run,omitempty"`
				Repack      bool   `json:"repack,omitempty"`
				GracePeriod string `json:"grace_period,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}

			var graceDuration time.Duration
			if in.GracePeriod != "" {
				d, err := time.ParseDuration(in.GracePeriod)
				if err != nil {
					return nil, err
				}
				graceDuration = d
			}

			return withRepo(stateDirProvider, func(repo *repository.Repository) (any, error) {
				result, err := repo.GC(repository.GCOptions{
					DryRun:      in.DryRun,
					Repack:      in.Repack,
					GracePeriod: graceDuration,
				})
				if err != nil {
					var warning *repository.GCCommittedWithWarningError
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

func toolMigrate(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_migrate",
		Description: "Upgrade an existing Spool repository state directory to a newer format version.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"from": map[string]any{
					"type":        "integer",
					"description": "Source repository format version",
				},
				"to": map[string]any{
					"type":        "integer",
					"description": "Target repository format version",
				},
			},
			"required": []string{"from", "to"},
		},
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				From int `json:"from"`
				To   int `json:"to"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.From <= 0 || in.To <= 0 {
				return nil, errors.New("both 'from' and 'to' must be positive integers")
			}
			stateDir, err := stateDirProvider()
			if err != nil {
				return nil, err
			}
			return repository.MigrateRepositoryFormat(stateDir, in.From, in.To)
		},
	}
}

func toolCherryPick(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_cherry_pick",
		Description: "Apply the exact graph delta of a historical commit onto a target branch without incorporating unrelated history.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"commit": map[string]any{
					"type":        "string",
					"description": "Source commit hash whose graph delta to transplant",
				},
				"target_branch": map[string]any{
					"type":        "string",
					"description": "Branch onto which to apply the transplanted delta",
				},
				"dry_run": map[string]any{
					"type":        "boolean",
					"description": "Simulate cherry-picking and report changes without modifying target branch",
				},
				"author": map[string]any{
					"type":        "string",
					"description": "Optional override commit author",
				},
				"message": map[string]any{
					"type":        "string",
					"description": "Optional override commit message",
				},
			},
			"required": []string{"commit", "target_branch"},
		},
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Commit       string `json:"commit"`
				TargetBranch string `json:"target_branch"`
				DryRun       bool   `json:"dry_run,omitempty"`
				Author       string `json:"author,omitempty"`
				Message      string `json:"message,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Commit == "" || in.TargetBranch == "" {
				return nil, errors.New("commit and target_branch are required")
			}
			return withRepo(stateDirProvider, func(repo *repository.Repository) (any, error) {
				result, err := repo.CherryPick(repository.CherryPickRequest{
					Commit:       in.Commit,
					TargetBranch: in.TargetBranch,
					DryRun:       in.DryRun,
					Author:       in.Author,
					Message:      in.Message,
				})
				if err != nil {
					var warning *repository.CherryPickCommittedWithWarningError
					if errors.As(err, &warning) || errors.Is(err, repository.ErrCherryPickConflicts) {
						return result, err
					}
					return nil, err
				}
				return result, nil
			})
		},
	}
}

func toolSchemaMigrate(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_schema_migrate",
		Description: "Stage a schema migration and conforming graph mutations atomically.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"branch": map[string]any{
					"type":        "string",
					"description": "Branch to migrate",
				},
				"schema_toml": map[string]any{
					"type":        "string",
					"description": "Inline TOML schema content or path to a schema.toml file",
				},
				"operations": map[string]any{
					"type":        "array",
					"description": "Optional conforming mutation operations to stage with the migration",
					"items": map[string]any{
						"type": "object",
					},
				},
			},
			"required": []string{"branch", "schema_toml"},
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
				if readErr != nil {
					return nil, fmt.Errorf("read schema file: %w", readErr)
				}
				schemaBytes = data
			} else if !errors.Is(err, os.ErrNotExist) {
				return nil, fmt.Errorf("stat schema file: %w", err)
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

func toolValidate(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_validate",
		Description: "Validate an immutable branch snapshot against the repository schema.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"branch": map[string]any{
					"type":        "string",
					"description": "Branch to validate",
				},
				"commit": map[string]any{
					"type":        "string",
					"description": "Optional reachable commit ID",
				},
			},
			"required": []string{"branch"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Branch string  `json:"branch"`
				Commit *string `json:"commit,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Branch == "" {
				return nil, errors.New("branch is required")
			}
			return withTool(stateDirProvider, func(tool *resolve.ResolveTool) (any, error) {
				return tool.SPLValidateSchema(ctx, resolve.SchemaValidationRequest{
					Selector: resolve.SnapshotSelector{
						Branch: in.Branch,
						Commit: in.Commit,
					},
				})
			})
		},
	}
}
