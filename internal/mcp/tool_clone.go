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
