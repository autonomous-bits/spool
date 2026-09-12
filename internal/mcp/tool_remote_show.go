package mcp

import (
	"context"
	"encoding/json"
	"os"

	"github.com/autonomous-bits/spool/internal/remote"
	"github.com/autonomous-bits/spool/internal/repository"
)

func toolRemoteShow(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_remote_show",
		Description: "Show the repository's configured Rack remote and version status.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Handler: func(ctx context.Context, _ json.RawMessage) (any, error) {
			return withRepo(stateDirProvider, func(repo *repository.Repository) (any, error) {
				cfg, ok, err := repo.Remote()
				if err != nil {
					return nil, err
				}
				if !ok {
					return nil, repository.ErrRemoteNotConfigured
				}
				repoID := cfg.RepoID
				if repoID == "" {
					repoID = cfg.WorkspaceID
				}
				res := map[string]any{
					"endpoint":      cfg.Endpoint,
					"tenantId":      cfg.TenantID,
					"workspaceId":   cfg.WorkspaceID,
					"repoId":        repoID,
					"authMode":      string(cfg.AuthMode),
					"versionStatus": "unreachable",
				}

				credential, _ := remote.ResolveCredential(cfg.WorkspaceOrRepoID(), cfg.AuthMode, remote.ResolveOptions{
					Keychain: remote.KeyringStore{},
					Getenv:   os.Getenv,
				})
				report, err := remote.NegotiateVersions(ctx, remote.NewClient(), cfg, credential.Value)
				if err != nil {
					res["message"] = remote.Redact(err.Error(), credential.Value, cfg.WorkspaceOrRepoID())
				} else {
					res["versionStatus"] = "negotiated"
					res["versions"] = report
				}
				return res, nil
			})
		},
	}
}
