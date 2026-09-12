package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/autonomous-bits/spool/internal/repository"
)

func toolWorkspaceAttach() Tool {
	return Tool{
		Name:        "spl_workspace_attach",
		Description: "Write a workspace manifest (.spl manifest) linking a directory to a central workspace.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"workspace": map[string]any{
					"type":        "string",
					"description": "Central workspace name to bind",
				},
				"repository_id": map[string]any{
					"type":        "string",
					"description": "Portable canonical repository identity",
				},
				"path": map[string]any{
					"type":        "string",
					"description": "Optional repository path (defaults to current working directory)",
				},
			},
			"required": []string{"workspace", "repository_id"},
		},
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Workspace    string `json:"workspace"`
				RepositoryID string `json:"repository_id"`
				Path         string `json:"path,omitempty"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Workspace == "" || in.RepositoryID == "" {
				return nil, errors.New("workspace and repository_id are required")
			}
			root, err := repository.WorkspaceStorageRoot()
			if err != nil {
				return nil, err
			}
			name, err := repository.ParseWorkspaceName(in.Workspace)
			if err != nil {
				return nil, err
			}
			registry, err := repository.LoadWorkspaceRegistry(root)
			if err != nil {
				return nil, err
			}
			workspace, exists := registry.Workspaces[name]
			if !exists {
				return nil, fmt.Errorf("%w: %q", repository.ErrWorkspaceNotRegistered, name)
			}

			targetPath := in.Path
			if targetPath == "" {
				targetPath, err = os.Getwd()
				if err != nil {
					return nil, err
				}
			}
			targetPath, err = filepath.Abs(targetPath)
			if err != nil {
				return nil, fmt.Errorf("make repository path absolute: %w", err)
			}

			if err := repository.WriteWorkspaceManifest(targetPath, repository.WorkspaceManifest{
				FormatVersion: repository.CurrentWorkspaceManifestVersion,
				RepositoryID:  in.RepositoryID,
				WorkspaceID:   workspace.ID,
			}); err != nil {
				return nil, err
			}

			return map[string]string{
				"workspaceId":  string(workspace.ID),
				"repositoryId": in.RepositoryID,
				"path":         targetPath,
			}, nil
		},
	}
}
