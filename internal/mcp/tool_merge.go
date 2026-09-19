package mcp

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/autonomous-bits/spool/internal/repository"
)

func toolMergePreview(rt *runtime) Tool {
	return Tool{
		Name:        "spl_merge_preview",
		Description: "Compute a deterministic three-way file-graph merge preview between two context-git branches.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"source": map[string]any{"type": "string", "description": "Source git branch"},
				"target": map[string]any{"type": "string", "description": "Target git branch"},
			},
			"required": []string{"source", "target"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
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
			session, err := rt.requireSession(ctx)
			if err != nil {
				return nil, err
			}
			return session.PreviewMerge(ctx, in.Source, in.Target)
		},
	}
}

func toolMergeApply(rt *runtime) Tool {
	return Tool{
		Name:        "spl_merge_apply",
		Description: "Apply a clean file-graph merge preview via a short-lived branch and PR.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"source":         map[string]any{"type": "string"},
				"target":         map[string]any{"type": "string"},
				"transaction_id": map[string]any{"type": "string"},
				"preview_id":     map[string]any{"type": "string"},
				"author":         map[string]any{"type": "string"},
				"message":        map[string]any{"type": "string"},
			},
			"required": []string{"source", "target", "transaction_id", "preview_id"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
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
			session, err := rt.requireSession(ctx)
			if err != nil {
				return nil, err
			}
			write, status, err := session.ApplyMerge(ctx, in.Source, in.Target, in.TransactionID, in.PreviewID, in.Author, in.Message)
			if err != nil {
				if errors.Is(err, repository.ErrMergeConflicted) {
					return status, err
				}
				return nil, err
			}
			return write, nil
		},
	}
}

func toolMergeConflicts(rt *runtime) Tool {
	return Tool{
		Name:        "spl_merge_conflicts",
		Description: "Inspect conflicts recorded for a file-graph merge transaction.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"transaction_id": map[string]any{"type": "string"},
			},
			"required": []string{"transaction_id"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				TransactionID string `json:"transaction_id"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			session, err := rt.requireSession(ctx)
			if err != nil {
				return nil, err
			}
			return session.InspectMerge(in.TransactionID)
		},
	}
}

func toolMergeResolve(rt *runtime) Tool {
	return Tool{
		Name:        "spl_merge_resolve",
		Description: "Resolve every conflict in a persisted file-graph merge using selections and optional overrides.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"transaction_id": map[string]any{"type": "string"},
				"preview_id":     map[string]any{"type": "string"},
				"selections": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"conflictId": map[string]any{"type": "string"},
							"choice":     map[string]any{"type": "string", "enum": []string{"source", "target"}},
						},
						"required": []string{"conflictId", "choice"},
					},
				},
				"overrides": map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
			},
			"required": []string{"transaction_id", "preview_id", "selections"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				TransactionID string                                `json:"transaction_id"`
				PreviewID     string                                `json:"preview_id"`
				Selections    []repository.MergeResolutionSelection `json:"selections"`
				Overrides     []repository.MutationOperation        `json:"overrides,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			session, err := rt.requireSession(ctx)
			if err != nil {
				return nil, err
			}
			if err := session.ResolveMerge(ctx, in.TransactionID, in.PreviewID, in.Selections, in.Overrides); err != nil {
				return nil, err
			}
			return map[string]any{"resolved": true}, nil
		},
	}
}

func toolMergeAbort(rt *runtime) Tool {
	return Tool{
		Name:        "spl_merge_abort",
		Description: "Abort a persisted file-graph merge transaction.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"transaction_id": map[string]any{"type": "string"},
			},
			"required": []string{"transaction_id"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				TransactionID string `json:"transaction_id"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			session, err := rt.requireSession(ctx)
			if err != nil {
				return nil, err
			}
			if err := session.AbortMerge(in.TransactionID); err != nil {
				return nil, err
			}
			return map[string]any{"aborted": true}, nil
		},
	}
}

func toolMergeFinalize(rt *runtime) Tool {
	return Tool{
		Name:        "spl_merge_finalize",
		Description: "Finalize a resolved file-graph merge via short-lived branch + PR.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"transaction_id": map[string]any{"type": "string"},
				"author":         map[string]any{"type": "string"},
				"message":        map[string]any{"type": "string"},
			},
			"required": []string{"transaction_id"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				TransactionID string `json:"transaction_id"`
				Author        string `json:"author"`
				Message       string `json:"message"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			session, err := rt.requireSession(ctx)
			if err != nil {
				return nil, err
			}
			return session.FinalizeMerge(ctx, in.TransactionID, in.Author, in.Message)
		},
	}
}
