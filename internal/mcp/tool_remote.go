package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/autonomous-bits/spool/internal/remote"
	"github.com/autonomous-bits/spool/internal/repository"
)

func toolRemoteSet(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_remote_set",
		Description: "Configure the repository's Rack remote endpoint, tenant, and auth mode.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"endpoint": map[string]any{
					"type":        "string",
					"description": "Rack remote HTTP(S) endpoint URL",
				},
				"auth_mode": map[string]any{
					"type":        "string",
					"description": "Authentication mode ('bearer' or 'api_key')",
					"enum":        []string{"bearer", "api_key"},
				},
				"tenant_id": map[string]any{
					"type":        "string",
					"description": "Optional logical Rack tenant identity",
				},
				"workspace_id": map[string]any{
					"type":        "string",
					"description": "Optional logical Rack workspace identity",
				},
				"repo_id": map[string]any{
					"type":        "string",
					"description": "Optional Rack repository identity",
				},
			},
			"required": []string{"endpoint", "auth_mode"},
		},
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Endpoint    string `json:"endpoint"`
				AuthMode    string `json:"auth_mode"`
				TenantID    string `json:"tenant_id,omitempty"`
				WorkspaceID string `json:"workspace_id,omitempty"`
				RepoID      string `json:"repo_id,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Endpoint == "" || in.AuthMode == "" {
				return nil, errors.New("endpoint and auth_mode are required")
			}
			return withRepo(stateDirProvider, func(repo *repository.Repository) (any, error) {
				cfg := repository.RemoteConfig{
					Endpoint:    in.Endpoint,
					AuthMode:    repository.RemoteAuthMode(in.AuthMode),
					TenantID:    in.TenantID,
					WorkspaceID: in.WorkspaceID,
					RepoID:      in.RepoID,
				}
				if err := repo.SetRemote(cfg); err != nil {
					return nil, err
				}
				return map[string]string{
					"endpoint":    cfg.Endpoint,
					"authMode":    string(cfg.AuthMode),
					"tenantId":    cfg.TenantID,
					"workspaceId": cfg.WorkspaceID,
					"repoId":      cfg.RepoID,
				}, nil
			})
		},
	}
}

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

func toolRemoteRemove(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_remote_remove",
		Description: "Remove the repository's configured Rack remote.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
		Handler: func(_ context.Context, _ json.RawMessage) (any, error) {
			return withRepo(stateDirProvider, func(repo *repository.Repository) (any, error) {
				_, existed, err := repo.Remote()
				if err != nil {
					return nil, err
				}
				if err := repo.RemoveRemote(); err != nil {
					return nil, err
				}
				return map[string]bool{"removed": existed}, nil
			})
		},
	}
}

func toolRemoteBranch(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_remote_branch",
		Description: "Inspect, list, create, or delete remote branches on the configured Rack remote.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"action": map[string]any{
					"type":        "string",
					"description": "Action to perform: 'list', 'default', 'create', or 'delete'",
					"enum":        []string{"list", "default", "create", "delete"},
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
					if err := repo.SetRemoteBranchTracking(in.Name, result.Name, result.HeadCommit); err != nil {
						return nil, fmt.Errorf("set remote branch tracking: %w", err)
					}
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

func toolPush(stateDirProvider func() (string, error)) Tool {
	return Tool{
		Name:        "spl_push",
		Description: "Push verified local commits for a branch to the repository's configured Rack remote. Returns rejection details if the remote branch has advanced (pull and merge locally before retrying).",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"branch": map[string]any{
					"type":        "string",
					"description": "Branch to push",
				},
				"base_commit": map[string]any{
					"type":        "string",
					"description": "Optional base commit known to remote",
				},
			},
			"required": []string{"branch"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Branch     string `json:"branch"`
				BaseCommit string `json:"base_commit,omitempty"`
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

				credential, err := remote.ResolveCredential(cfg.WorkspaceOrRepoID(), cfg.AuthMode, remote.ResolveOptions{
					Keychain: remote.KeyringStore{},
					Getenv:   os.Getenv,
				})
				if err != nil {
					return nil, fmt.Errorf("resolve credential: %w", err)
				}

				client := remote.NewClient()
				pack, err := repo.BuildPushPack(ctx, in.Branch, in.BaseCommit)
				if err != nil {
					if errors.Is(err, repository.ErrNothingToPush) {
						return map[string]any{
							"branch":  in.Branch,
							"pushed":  false,
							"message": "nothing to push",
						}, nil
					}
					return nil, err
				}

				if len(pack.AssetHashes) > 0 {
					negRes, negErr := remote.NegotiateAssets(ctx, client, cfg, credential.Value, remote.AssetNegotiationRequest{
						Hashes: pack.AssetHashes,
					})
					if negErr != nil {
						return nil, fmt.Errorf("negotiate assets: %w", negErr)
					}
					for _, missingHash := range negRes.Missing {
						reader, _, _, readErr := repo.ReadAsset(ctx, in.Branch, missingHash)
						if readErr != nil {
							return nil, fmt.Errorf("open missing asset %s: %w", missingHash, readErr)
						}
						uploadErr := remote.UploadAsset(ctx, client, cfg, credential.Value, missingHash, "application/octet-stream", reader)
						_ = reader.Close()
						if uploadErr != nil {
							return nil, fmt.Errorf("upload asset %s: %w", missingHash, uploadErr)
						}
					}
				}

				remoteCommits := make([]remote.PushCommitRecord, len(pack.Commits))
				for i, commit := range pack.Commits {
					remoteCommits[i] = remote.PushCommitRecord{ID: commit.ID, Commit: commit.Commit}
				}

				req := remote.PushRequest{
					Branch:       pack.Branch,
					BaseCommit:   pack.BaseCommit,
					TargetCommit: pack.TargetCommit,
					Commits:      remoteCommits,
					PackHash:     pack.PackHash,
					PackFormat:   uint32(pack.PackFormat),
					PackData:     pack.PackData,
					AssetHashes:  pack.AssetHashes,
				}
				result, err := remote.Push(ctx, client, cfg, credential.Value, req)
				if err != nil {
					var nffErr *remote.NonFastForwardError
					if errors.As(err, &nffErr) {
						return map[string]any{
							"branch":        in.Branch,
							"pushed":        false,
							"rejected":      true,
							"actualHead":    nffErr.ActualHead,
							"message":       nffErr.Guidance,
							"correlationId": nffErr.CorrelationID,
						}, nil
					}
					return nil, err
				}
				if err := repo.SetRemoteBranchTracking(in.Branch, result.Branch, result.HeadCommit); err != nil {
					return nil, fmt.Errorf("update remote branch tracking: %w", err)
				}
				return result, nil
			})
		},
	}
}

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
					var diverged *remote.DivergedError
					if errors.As(err, &diverged) {
						return map[string]any{
							"branch":        in.Branch,
							"pulled":        false,
							"diverged":      true,
							"actualHead":    diverged.ActualHead,
							"message":       diverged.Guidance,
							"correlationId": diverged.CorrelationID,
						}, nil
					}
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

func toolClone() Tool {
	return Tool{
		Name:        "spl_clone",
		Description: "Clone a remote workspace from Spool Rack into a new local directory.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"endpoint": map[string]any{
					"type":        "string",
					"description": "Rack remote HTTP(S) endpoint URL",
				},
				"workspace_id": map[string]any{
					"type":        "string",
					"description": "Logical Rack workspace identity",
				},
				"tenant_id": map[string]any{
					"type":        "string",
					"description": "Optional logical Rack tenant identity",
				},
				"directory": map[string]any{
					"type":        "string",
					"description": "Destination directory path (defaults to workspace_id)",
				},
				"branch": map[string]any{
					"type":        "string",
					"description": "Remote branch to clone (defaults to remote default branch)",
				},
				"auth_mode": map[string]any{
					"type":        "string",
					"description": "Authentication mode ('bearer' or 'api_key', defaults to 'bearer')",
				},
			},
			"required": []string{"endpoint", "workspace_id"},
		},
		Handler: func(ctx context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Endpoint    string `json:"endpoint"`
				WorkspaceID string `json:"workspace_id"`
				TenantID    string `json:"tenant_id,omitempty"`
				Directory   string `json:"directory,omitempty"`
				Branch      string `json:"branch,omitempty"`
				AuthMode    string `json:"auth_mode,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Endpoint == "" || in.WorkspaceID == "" {
				return nil, errors.New("endpoint and workspace_id are required")
			}
			if in.AuthMode == "" {
				in.AuthMode = "bearer"
			}
			cfg := repository.RemoteConfig{
				Endpoint:    in.Endpoint,
				WorkspaceID: in.WorkspaceID,
				TenantID:    in.TenantID,
				AuthMode:    repository.RemoteAuthMode(in.AuthMode),
			}
			if err := cfg.Validate(); err != nil {
				return nil, err
			}
			targetDir := in.Directory
			if targetDir == "" {
				targetDir = in.WorkspaceID
			}
			absDestDir, err := filepath.Abs(targetDir)
			if err != nil {
				return nil, err
			}

			if info, statErr := os.Stat(absDestDir); statErr == nil {
				if !info.IsDir() {
					return nil, fmt.Errorf("destination path %q already exists and is not a directory", absDestDir)
				}
				entries, readErr := os.ReadDir(absDestDir)
				if readErr != nil {
					return nil, fmt.Errorf("inspect destination directory %q: %w", absDestDir, readErr)
				}
				if len(entries) > 0 {
					return nil, fmt.Errorf("destination path %q already exists and is not an empty directory", absDestDir)
				}
			} else if !os.IsNotExist(statErr) {
				return nil, fmt.Errorf("inspect destination path %q: %w", absDestDir, statErr)
			}

			credential, _ := remote.ResolveCredential(cfg.WorkspaceOrRepoID(), cfg.AuthMode, remote.ResolveOptions{
				Keychain: remote.KeyringStore{},
				Getenv:   os.Getenv,
			})

			client := remote.NewClient()
			cloneRes, err := remote.Clone(ctx, client, cfg, credential.Value, in.Branch)
			if err != nil {
				return nil, err
			}

			stateDir := filepath.Join(absDestDir, ".spl")
			repo, commitsInstalled, err := repository.InitializeClonedRepository(stateDir, cfg, cloneRes.Branch, cloneRes.HeadCommit, cloneRes.Packs)
			if err != nil {
				return nil, fmt.Errorf("initialize cloned workspace: %w", err)
			}
			if closeErr := repo.Close(); closeErr != nil {
				return nil, fmt.Errorf("finalize cloned workspace: %w", closeErr)
			}

			return map[string]any{
				"cloned":           true,
				"directory":        absDestDir,
				"endpoint":         cfg.Endpoint,
				"workspaceId":      cfg.WorkspaceID,
				"branch":           cloneRes.Branch,
				"headCommit":       cloneRes.HeadCommit,
				"commitsInstalled": commitsInstalled,
				"empty":            cloneRes.Empty,
			}, nil
		},
	}
}
