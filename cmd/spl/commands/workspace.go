package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/spf13/cobra"
)

const maxWorkspaceIdentityAttempts = 8

// NewWorkspaceCommandDefault creates the explicit central workspace
// provisioning commands.
func NewWorkspaceCommandDefault() *cobra.Command {
	return NewWorkspaceCommand(repository.WorkspaceStorageRoot)
}

func NewWorkspaceCommand(registryRoot func() (string, error)) *cobra.Command {
	if registryRoot == nil {
		registryRoot = repository.WorkspaceStorageRoot
	}
	command := &cobra.Command{
		Use:          "workspace",
		Short:        "Provision central detached workspaces",
		SilenceUsage: true,
	}
	command.AddCommand(newWorkspaceInitCommand(registryRoot), newWorkspaceAttachCommand(registryRoot))
	return command
}

func newWorkspaceInitCommand(registryRoot func() (string, error)) *cobra.Command {
	return &cobra.Command{
		Use:          "init <name>",
		Short:        "Create a central detached workspace",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(command *cobra.Command, args []string) error {
			root, err := registryRoot()
			if err != nil {
				return err
			}
			name, err := repository.ParseWorkspaceName(args[0])
			if err != nil {
				return err
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
					if workspaceIdentityAvailable(*registry, id, stateDir) {
						break
					}
					id = ""
				}
				if id == "" {
					return fmt.Errorf("generate workspace identity: could not find a unique ID after %d attempts", maxWorkspaceIdentityAttempts)
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
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(struct {
				Name     string `json:"name"`
				ID       string `json:"id"`
				StateDir string `json:"stateDir"`
			}{Name: string(name), ID: string(created.ID), StateDir: created.StateDir})
		},
	}
}

func workspaceIdentityAvailable(registry repository.WorkspaceRegistry, id repository.WorkspaceID, stateDir string) bool {
	for _, workspace := range registry.Workspaces {
		if workspace.ID == id || workspace.StateDir == stateDir {
			return false
		}
	}
	return true
}

func newWorkspaceAttachCommand(registryRoot func() (string, error)) *cobra.Command {
	var workspaceName, repositoryID string
	command := &cobra.Command{
		Use:          "attach [path]",
		Short:        "Write a workspace manifest for a repository",
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(command *cobra.Command, args []string) error {
			root, err := registryRoot()
			if err != nil {
				return err
			}
			name, err := repository.ParseWorkspaceName(workspaceName)
			if err != nil {
				return err
			}
			registry, err := repository.LoadWorkspaceRegistry(root)
			if err != nil {
				return err
			}
			workspace, exists := registry.Workspaces[name]
			if !exists {
				return fmt.Errorf("%w: %q", repository.ErrWorkspaceNotRegistered, name)
			}
			path := ""
			if len(args) == 0 {
				path, err = os.Getwd()
				if err != nil {
					return err
				}
			} else {
				path = args[0]
			}
			path, err = filepath.Abs(path)
			if err != nil {
				return fmt.Errorf("make repository path absolute: %w", err)
			}
			if err := repository.WriteWorkspaceManifest(path, repository.WorkspaceManifest{
				FormatVersion: repository.CurrentWorkspaceManifestVersion,
				RepositoryID:  repositoryID,
				WorkspaceID:   workspace.ID,
			}); err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(struct {
				WorkspaceID  string `json:"workspaceId"`
				RepositoryID string `json:"repositoryId"`
			}{WorkspaceID: string(workspace.ID), RepositoryID: repositoryID})
		},
	}
	command.Flags().StringVar(&workspaceName, "workspace", "", "central workspace to bind")
	command.Flags().StringVar(&repositoryID, "repository-id", "", "portable canonical repository identity")
	_ = command.MarkFlagRequired("workspace")
	_ = command.MarkFlagRequired("repository-id")
	return command
}
