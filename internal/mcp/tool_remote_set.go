package mcp

import (
	"context"
	"encoding/json"
	"errors"

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
