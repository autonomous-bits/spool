package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/autonomous-bits/spool/internal/remote"
	"github.com/autonomous-bits/spool/internal/repository"
)

func toolPull(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_pull",
		Description: "Pull new commits for a branch from the repository's configured Rack remote and fast-forward local history.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"branch": map[string]any{
					"type":        "string",
					"description": "Local branch to pull",
				},
			},
			"required": []string{"branch"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Branch string `json:"branch"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Branch == "" {
				return nil, errors.New("branch is required")
			}

			return withRepo(stateDirProvider, func(repo *repository.Repository) (any, error) {
				cfg, ok, err := repo.Remote()
				if err != nil {
					return nil, err
				}
				if !ok {
					return nil, repository.ErrRemoteNotConfigured
				}

				local, err := repo.BuildPushPack(ctx, in.Branch, "")
				if err != nil {
					return nil, fmt.Errorf("determine local branch head: %w", err)
				}

				credential, err := remote.ResolveCredential(cfg.WorkspaceOrRepoID(), cfg.AuthMode, remote.ResolveOptions{
					Keychain: remote.KeyringStore{},
					Getenv:   os.Getenv,
				})
				if err != nil {
					return nil, fmt.Errorf("resolve credential: %w", err)
				}

				pulled, err := remote.Pull(ctx, remote.NewClient(), cfg, credential.Value, in.Branch, local.TargetCommit)
				if err != nil {
					return nil, err
				}
				if pulled.UpToDate {
					return map[string]any{
						"branch":     in.Branch,
						"pulled":     false,
						"upToDate":   true,
						"headCommit": pulled.HeadCommit,
						"message":    "up to date",
					}, nil
				}

				installed, err := repo.InstallPullPack(ctx, in.Branch, pulled.Packs, pulled.HeadCommit)
				if err != nil {
					return nil, fmt.Errorf("install pulled pack: %w", err)
				}

				return map[string]any{
					"branch":           installed.Branch,
					"pulled":           true,
					"commitsInstalled": installed.CommitsInstalled,
					"headCommit":       pulled.HeadCommit,
				}, nil
			})
		},
	}
}
