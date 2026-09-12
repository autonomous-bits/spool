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

func toolRemoteBranch(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_remote_branch",
		Description: "Manage branches on the repository's configured Rack remote: list, create, get default, or delete.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"description": "Action to perform: 'list', 'create', 'default', or 'delete'",
					"enum":        []string{"list", "create", "default", "delete"},
				},
				"name": map[string]any{
					"type":        "string",
					"description": "Remote branch name (required for create and delete)",
				},
				"from_branch": map[string]any{
					"type":        "string",
					"description": "Existing remote branch to use as source (for create)",
				},
				"from_commit": map[string]any{
					"type":        "string",
					"description": "Existing remote commit to use as source (for create)",
				},
			},
			"required": []string{"action"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Action     string `json:"action"`
				Name       string `json:"name,omitempty"`
				FromBranch string `json:"from_branch,omitempty"`
				FromCommit string `json:"from_commit,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}

			return withRepo(stateDirProvider, func(repo *repository.Repository) (any, error) {
				cfg, ok, err := repo.Remote()
				if err != nil {
					return nil, err
				}
				if !ok {
					return nil, repository.ErrRemoteNotConfigured
				}
				credential, err := remote.ResolveCredential(cfg.WorkspaceOrRepoID(), cfg.AuthMode, remote.ResolveOptions{
					Keychain: remote.KeyringStore{},
					Getenv:   os.Getenv,
				})
				if err != nil {
					return nil, fmt.Errorf("resolve credential: %w", err)
				}
				client := remote.NewClient()

				switch in.Action {
				case "list":
					result, err := remote.ListBranches(ctx, client, cfg, credential.Value)
					if err != nil {
						return nil, err
					}
					return result, nil

				case "default":
					result, err := remote.DefaultBranch(ctx, client, cfg, credential.Value)
					if err != nil {
						return nil, err
					}
					return result, nil

				case "create":
					if in.Name == "" {
						return nil, errors.New("name is required for create")
					}
					result, err := remote.CreateBranch(ctx, client, cfg, credential.Value, remote.BranchCreateRequest{
						Name:         in.Name,
						SourceBranch: in.FromBranch,
						SourceCommit: in.FromCommit,
					})
					if err != nil {
						return nil, err
					}
					_ = repo.SetRemoteBranchTracking(in.Name, result.Name, result.HeadCommit)
					return result, nil

				case "delete":
					if in.Name == "" {
						return nil, errors.New("name is required for delete")
					}
					if err := remote.DeleteBranch(ctx, client, cfg, credential.Value, in.Name); err != nil {
						return nil, err
					}
					return map[string]any{"name": in.Name, "deleted": true}, nil

				default:
					return nil, fmt.Errorf("unsupported action: %q", in.Action)
				}
			})
		},
	}
}
