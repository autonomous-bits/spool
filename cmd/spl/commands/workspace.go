package commands

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	workspacepkg "github.com/autonomous-bits/spool/internal/workspace"
	"github.com/spf13/cobra"
)

type workspacePathResult struct {
	Workspace string `json:"workspace"`
	Attached  string `json:"attached,omitempty"`
	Detached  string `json:"detached,omitempty"`
}

// NewWorkspaceCommandDefault creates the workspace command using the default
// detached-storage root provider.
func NewWorkspaceCommandDefault() *cobra.Command {
	return NewWorkspaceCommand(workspacepkg.StorageRoot)
}

// NewWorkspaceCommand creates the workspace command group.
func NewWorkspaceCommand(registryRoot func() (string, error)) *cobra.Command {
	if registryRoot == nil {
		registryRoot = workspacepkg.StorageRoot
	}

	command := &cobra.Command{
		Use:          "workspace",
		Short:        "Manage detached workspaces",
		Long:         "Create, attach, detach, inspect, and resolve detached workspaces stored in the central workspace registry.",
		Example:      "  spl workspace init ecommerce-platform\n  spl workspace attach --workspace ecommerce-platform\n  spl workspace list\n  spl workspace current",
		SilenceUsage: true,
	}
	command.AddCommand(
		newWorkspaceInitCommand(registryRoot),
		newWorkspaceAttachCommand(registryRoot),
		newWorkspaceDetachCommand(registryRoot),
		newWorkspaceListCommand(registryRoot),
		newWorkspaceCurrentCommand(registryRoot),
	)
	return command
}

func newWorkspaceInitCommand(registryRoot func() (string, error)) *cobra.Command {
	return &cobra.Command{
		Use:          "init <name>",
		Short:        "Create a detached workspace",
		Long:         "Create a named detached workspace, persist it in the workspace registry, and write the created workspace as JSON.",
		Example:      "  spl workspace init ecommerce-platform",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(command *cobra.Command, args []string) error {
			root, err := registryRoot()
			if err != nil {
				return err
			}
			name, err := workspacepkg.ParseName(args[0])
			if err != nil {
				return err
			}
			request, err := workspacepkg.NewCreateRequest(name)
			if err != nil {
				return err
			}
			stateDir, err := workspacepkg.RepositoryPath(root, request.ID)
			if err != nil {
				return err
			}
			created := workspacepkg.Workspace{
				ID:        request.ID,
				Name:      string(name),
				StateDir:  stateDir,
				CreatedAt: time.Now().UTC(),
				Paths:     nil,
			}
			if err := workspacepkg.UpdateRegistry(root, func(registry *workspacepkg.Registry) error {
				if _, exists := registry.Workspaces[name]; exists {
					return fmt.Errorf("workspace %q already exists", name)
				}
				registry.Workspaces[name] = created
				return nil
			}); err != nil {
				return err
			}
			created.Paths = []string{}
			return json.NewEncoder(command.OutOrStdout()).Encode(created)
		},
	}
}

func newWorkspaceAttachCommand(registryRoot func() (string, error)) *cobra.Command {
	var workspaceName string
	command := &cobra.Command{
		Use:          "attach [path]",
		Short:        "Attach a repository path to a workspace",
		Long:         "Attach a repository path, defaulting to the current working directory, to an existing detached workspace and write the attachment result as JSON.",
		Example:      "  spl workspace attach --workspace ecommerce-platform\n  spl workspace attach --workspace ecommerce-platform ~/repos/order-service",
		Args:         cobra.MaximumNArgs(1),
		SilenceUsage: true,
		RunE: func(command *cobra.Command, args []string) error {
			root, err := registryRoot()
			if err != nil {
				return err
			}
			name, err := workspacepkg.ParseName(workspaceName)
			if err != nil {
				return err
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
			if err := workspacepkg.AttachPath(root, name, path); err != nil {
				return err
			}
			attachedPath, err := workspacepkg.CanonicalPath(path)
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(workspacePathResult{
				Workspace: string(name),
				Attached:  attachedPath,
			})
		},
	}
	command.Flags().StringVar(&workspaceName, "workspace", "", "existing workspace to attach the path to")
	_ = command.MarkFlagRequired("workspace")
	return command
}

func newWorkspaceDetachCommand(registryRoot func() (string, error)) *cobra.Command {
	return &cobra.Command{
		Use:          "detach <path>",
		Short:        "Detach a repository path from its workspace",
		Long:         "Detach a previously attached repository path from whichever workspace currently owns it and write the detachment result as JSON.",
		Example:      "  spl workspace detach ~/repos/infrastructure",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(command *cobra.Command, args []string) error {
			root, err := registryRoot()
			if err != nil {
				return err
			}
			result, err := detachWorkspacePath(root, args[0])
			if err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(result)
		},
	}
}

func newWorkspaceListCommand(registryRoot func() (string, error)) *cobra.Command {
	return &cobra.Command{
		Use:          "list",
		Short:        "List registered workspaces",
		Long:         "List all registered detached workspaces in deterministic alphabetical order as JSON.",
		Example:      "  spl workspace list",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			root, err := registryRoot()
			if err != nil {
				return err
			}
			registry, err := workspacepkg.LoadRegistry(root)
			if err != nil {
				return err
			}
			names := make([]workspacepkg.Name, 0, len(registry.Workspaces))
			for name := range registry.Workspaces {
				names = append(names, name)
			}
			sort.Slice(names, func(i, j int) bool { return names[i] < names[j] })
			result := make([]workspacepkg.Workspace, 0, len(names))
			for _, name := range names {
				entry := registry.Workspaces[name]
				if entry.Paths == nil {
					entry.Paths = []string{}
				}
				result = append(result, entry)
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(result)
		},
	}
}

func newWorkspaceCurrentCommand(registryRoot func() (string, error)) *cobra.Command {
	return &cobra.Command{
		Use:          "current",
		Short:        "Show the workspace owning the current directory",
		Long:         "Resolve the registered workspace that owns the current working directory and write it as JSON.",
		Example:      "  spl workspace current",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			root, err := registryRoot()
			if err != nil {
				return err
			}
			workingDirectory, err := os.Getwd()
			if err != nil {
				return err
			}
			match, err := workspacepkg.FindWorkspace(root, workingDirectory)
			if err != nil {
				if errors.Is(err, workspacepkg.ErrWorkspaceNotFound) {
					return fmt.Errorf("no workspace is registered for the current directory: %w", err)
				}
				return err
			}
			if match.Workspace.Paths == nil {
				match.Workspace.Paths = []string{}
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(match.Workspace)
		},
	}
}

func detachWorkspacePath(root, path string) (workspacePathResult, error) {
	canonicalPath, err := workspacepkg.CanonicalPath(path)
	if err != nil {
		return workspacePathResult{}, err
	}
	var detachedWorkspace workspacepkg.Name
	err = workspacepkg.UpdateRegistry(root, func(registry *workspacepkg.Registry) error {
		for name, entry := range registry.Workspaces {
			remainingPaths := make([]string, 0, len(entry.Paths))
			found := false
			for _, attachedPath := range entry.Paths {
				if attachedPath == canonicalPath {
					found = true
					continue
				}
				remainingPaths = append(remainingPaths, attachedPath)
			}
			if !found {
				continue
			}
			entry.Paths = remainingPaths
			registry.Workspaces[name] = entry
			detachedWorkspace = name
			return nil
		}
		return fmt.Errorf("path %q is not attached to any workspace", canonicalPath)
	})
	if err != nil {
		return workspacePathResult{}, err
	}
	return workspacePathResult{
		Workspace: string(detachedWorkspace),
		Detached:  canonicalPath,
	}, nil
}
