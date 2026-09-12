package mcp

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/resolve"
)

func toolBranchList(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_branch_list",
		Description: "List all local branches in the Spool repository.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Handler: func(_ context.Context, _ json.RawMessage) (any, error) {
			return withRepo(stateDirProvider, func(repo *repository.Repository) (any, error) {
				return repo.ListBranches()
			})
		},
	}
}

func toolBranchCreate(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_branch_create",
		Description: "Create a new branch from an existing source branch or commit.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Name of the new branch to create",
				},
				"from_branch": map[string]any{
					"type":        "string",
					"description": "Existing branch to use as source",
				},
				"from_commit": map[string]any{
					"type":        "string",
					"description": "Existing commit ID to use as source",
				},
			},
			"required": []string{"name"},
		},
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Name       string `json:"name"`
				FromBranch string `json:"from_branch,omitempty"`
				FromCommit string `json:"from_commit,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Name == "" {
				return nil, errors.New("name is required")
			}
			return withRepo(stateDirProvider, func(repo *repository.Repository) (any, error) {
				return repo.CreateBranch(in.Name, repository.BranchSource{
					Branch: in.FromBranch,
					Commit: in.FromCommit,
				})
			})
		},
	}
}

func toolBranchDelete(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_branch_delete",
		Description: "Delete an inactive, non-default local branch.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Branch name to delete",
				},
			},
			"required": []string{"name"},
		},
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Name == "" {
				return nil, errors.New("name is required")
			}
			return withRepo(stateDirProvider, func(repo *repository.Repository) (any, error) {
				return repo.DeleteBranch(in.Name)
			})
		},
	}
}

func toolSwitch(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_switch",
		Description: "Switch the repository's active branch.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Branch name to switch to",
				},
			},
			"required": []string{"name"},
		},
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Name == "" {
				return nil, errors.New("name is required")
			}
			return withRepo(stateDirProvider, func(repo *repository.Repository) (any, error) {
				return repo.SwitchBranch(in.Name)
			})
		},
	}
}

func toolBranchesContaining(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_branches_containing",
		Description: "Find all repository branches containing a specific entity or snapshot ID.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"entity_id": map[string]any{
					"type":        "string",
					"description": "Stable entity or node ID to find",
				},
				"snapshot_id": map[string]any{
					"type":        "string",
					"description": "Exact graph snapshot object ID to find",
				},
				"natural_key": map[string]any{
					"type":        "string",
					"description": "Schema-defined natural key to find",
				},
				"continuation": map[string]any{
					"type":        "string",
					"description": "Optional pagination continuation token",
				},
				"budget": budgetSchema,
			},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				EntityID     string       `json:"entity_id,omitempty"`
				SnapshotID   string       `json:"snapshot_id,omitempty"`
				NaturalKey   string       `json:"natural_key,omitempty"`
				Continuation string       `json:"continuation,omitempty"`
				Budget       *budgetInput `json:"budget,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			return withTool(stateDirProvider, func(tool *resolve.ResolveTool) (any, error) {
				return tool.SPLBranchesContainingPage(ctx, resolve.BranchesContainingRequest{
					Selector: resolve.ContainmentSelector{
						EntityID:   in.EntityID,
						SnapshotID: repository.ObjectID(in.SnapshotID),
						NaturalKey: in.NaturalKey,
					},
					ContinuationToken: in.Continuation,
					Budget:            in.Budget.toBudget(),
				})
			})
		},
	}
}
