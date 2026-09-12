package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/autonomous-bits/spool/internal/repository"
)

const maxWorkspaceIdentityAttempts = 8

func toolWorkspaceInit() Tool {
	return Tool{
		Name:        "spl_workspace_init",
		Description: "Create a central detached workspace in the user's workspace storage root.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name": map[string]any{
					"type":        "string",
					"description": "Unique workspace name",
				},
			},
			"required": []string{"name"},
		},
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var in struct {
				Name string `json:"name"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, err
			}
			if in.Name == "" {
				return nil, errors.New("name is required")
			}
			root, err := repository.WorkspaceStorageRoot()
			if err != nil {
				return nil, err
			}
			name, err := repository.ParseWorkspaceName(in.Name)
			if err != nil {
				return nil, err
			}

			var created repository.Workspace
			err = repository.UpdateWorkspaceRegistry(root, func(registry *repository.WorkspaceRegistry) error {
				if _, exists := registry.Workspaces[name]; exists {
					return fmt.Errorf("%w: %q", repository.ErrWorkspaceExists, name)
				}
				var id repository.WorkspaceID
				var stateDir string
				for attempt := 0; attempt < maxWorkspaceIdentityAttempts; attempt++ {
					id, err = repository.NewWorkspaceID()
					if err != nil {
						return err
					}
					stateDir, err = repository.RepositoryWorkspacePath(root, id)
					if err != nil {
						return err
					}
					conflict := false
					for _, ws := range registry.Workspaces {
						if ws.ID == id || ws.StateDir == stateDir {
							conflict = true
							break
						}
					}
					if !conflict {
						break
					}
					id = ""
				}
				if id == "" {
					return fmt.Errorf("could not find a unique workspace ID after %d attempts", maxWorkspaceIdentityAttempts)
				}
				repo, err := repository.InitializeRepository(stateDir)
				if err != nil {
					return fmt.Errorf("initialize workspace state: %w", err)
				}
				if err := repo.Close(); err != nil {
					return fmt.Errorf("close initialized workspace state: %w", err)
				}
				created = repository.Workspace{ID: id, StateDir: stateDir}
				registry.Workspaces[name] = created
				return nil
			})
			if err != nil {
				return nil, err
			}
			return map[string]string{
				"name":     string(name),
				"id":       string(created.ID),
				"stateDir": created.StateDir,
			}, nil
		},
	}
}
