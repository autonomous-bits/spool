package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/autonomous-bits/spool/internal/ctxgit"
	"github.com/autonomous-bits/spool/internal/repository"
)

func toolContextExport(rt *runtime) Tool {
	return Tool{
		Name: "spl_context_export",
		Description: "Best-effort one-shot migrate-once export of a leftover .spl branch snapshot into the bound context git remote. " +
			"Not sync. Kept: nodes, edges, schema, assets. Dropped: packs, Rack remotes, reflogs, merge leases, projections. " +
			"Lossy is OK. Re-run is overwrite-at-own-risk. Requires .spool/context.toml. Unbound workspaces are refused.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"from": map[string]any{
					"type":        "string",
					"description": "Leftover .spl state directory (defaults to <workspace>/.spl)",
				},
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
				From    string `json:"from"`
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
			workspace, err := rt.workspaceDir()
			if err != nil {
				return nil, err
			}
			stateDir := in.From
			if stateDir == "" {
				stateDir = filepath.Join(workspace, ".spl")
			}
			repo, err := repository.OpenRepository(stateDir)
			if err != nil {
				return nil, fmt.Errorf("open leftover .spl at %s: %w", stateDir, err)
			}
			defer func() { _ = repo.Close() }()
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
		},
	}
}
