package mcp

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/autonomous-bits/spool/internal/repository"
)

func toolMergePreview(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_merge_preview",
		Description: "Compute a deterministic three-way merge preview between source and target branches.",
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
			},
			"required": []string{"source", "target"},
		},
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Source string `json:"source"`
				Target string `json:"target"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Source == "" || in.Target == "" {
				return nil, errors.New("source and target are required")
			}
			return withRepo(stateDirProvider, func(repo *repository.Repository) (any, error) {
				return repo.PreviewMerge(in.Source, in.Target)
			})
		},
	}
}

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

func toolMergeResolve(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_merge_resolve",
		Description: "Resolve every conflict in a persisted merge transaction using resolution selections and optional mutation overrides.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"target": map[string]any{
					"type":        "string",
					"description": "Target branch with the conflicted merge",
				},
				"transaction_id": map[string]any{
					"type":        "string",
					"description": "Owning merge transaction identifier",
				},
				"preview_id": map[string]any{
					"type":        "string",
					"description": "Persisted preview identifier",
				},
				"selections": map[string]any{
					"type":        "array",
					"description": "Array of merge resolution selections specifying conflictId and choice ('source' or 'target')",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"conflictId": map[string]any{
								"type":        "string",
								"description": "Conflict identifier from spl_merge_conflicts",
							},
							"choice": map[string]any{
								"type":        "string",
								"enum":        []string{"source", "target"},
								"description": "Resolution choice: 'source' or 'target'",
							},
						},
						"required": []string{"conflictId", "choice"},
					},
				},
				"overrides": map[string]any{
					"type":        "array",
					"description": "Optional array of mutation operations providing custom resolutions",
					"items": map[string]any{
						"type": "object",
					},
				},
			},
			"required": []string{"target", "transaction_id", "preview_id", "selections"},
		},
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Target        string                                `json:"target"`
				TransactionID string                                `json:"transaction_id"`
				PreviewID     string                                `json:"preview_id"`
				Selections    []repository.MergeResolutionSelection `json:"selections"`
				Overrides     []repository.MutationOperation        `json:"overrides,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Target == "" || in.TransactionID == "" || in.PreviewID == "" {
				return nil, errors.New("target, transaction_id, and preview_id are required")
			}
			return withRepo(stateDirProvider, func(repo *repository.Repository) (any, error) {
				if err := repo.ResolveConflictedMerge(repository.ResolveConflictedMergeRequest{
					TargetBranch:  in.Target,
					TransactionID: in.TransactionID,
					PreviewID:     repository.ObjectID(in.PreviewID),
					Selections:    in.Selections,
					Overrides:     in.Overrides,
				}); err != nil {
					return nil, err
				}
				return map[string]any{"resolved": true}, nil
			})
		},
	}
}

func toolMergeAbort(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_merge_abort",
		Description: "Abort an active merge transaction and discard its working state.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"target": map[string]any{
					"type":        "string",
					"description": "Target branch of the merge",
				},
				"transaction_id": map[string]any{
					"type":        "string",
					"description": "Transaction ID to abort",
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
				if err := repo.AbortMergeTransaction(in.Target, in.TransactionID); err != nil {
					return nil, err
				}
				return map[string]any{"aborted": true}, nil
			})
		},
	}
}

func toolMergeFinalize(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_merge_finalize",
		Description: "Finalize an active merge transaction after resolving conflicts.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"target": map[string]any{
					"type":        "string",
					"description": "Target branch of the merge",
				},
				"transaction_id": map[string]any{
					"type":        "string",
					"description": "Transaction ID to finalize",
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
				commit, err := repo.FinalizeMergeTransaction(in.Target, in.TransactionID)
				if err != nil {
					return nil, err
				}
				return map[string]any{"commit": commit}, nil
			})
		},
	}
}
