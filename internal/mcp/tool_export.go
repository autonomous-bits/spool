package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/autonomous-bits/spool/internal/ctxgit"
	"github.com/autonomous-bits/spool/internal/repository"
)

func toolContextExport(rt *runtime) Tool {
	return Tool{
		Name: "spl_context_export",
		Description: "Best-effort one-shot migrate-once export of a local .spl branch snapshot into the bound context git remote. " +
			"Not sync. Kept: nodes, edges, schema, assets (LFS ≥512 KiB). Dropped: packs, Rack remotes, reflogs, merge leases, projections. " +
			"Lossy is OK. Re-run is overwrite-at-own-risk. Requires .spool/context.toml. Unbound workspaces are refused. " +
			"One batch → one commit on a short-lived branch → PR. Does not configure Rack remotes.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"branch": map[string]any{
					"type":        "string",
					"description": "Local .spl branch to export (defaults to the active branch)",
				},
				"author": map[string]any{
					"type":        "string",
					"description": "Git author for the export commit",
				},
				"message": map[string]any{
					"type":        "string",
					"description": "Commit/PR title (migrate-once, overwrite-at-own-risk)",
				},
			},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Branch  string `json:"branch"`
				Author  string `json:"author"`
				Message string `json:"message"`
			}
			if len(args) > 0 && string(args) != "null" {
				if err := json.Unmarshal(args, &in); err != nil {
					return nil, err
				}
			}
			session, err := rt.requireSession(ctx)
			if err != nil {
				return nil, err
			}
			return withRepo(rt.stateDir, func(repo *repository.Repository) (any, error) {
				result, err := session.Export(ctx, ctxgit.ExportRequest{
					Repo:    repo,
					Branch:  in.Branch,
					Author:  in.Author,
					Message: in.Message,
				})
				if err != nil {
					if errors.Is(err, ctxgit.ErrUnbound) {
						return nil, err
					}
					return nil, fmt.Errorf("export/migrate-once failed: %w", err)
				}
				return result, nil
			})
		},
	}
}
