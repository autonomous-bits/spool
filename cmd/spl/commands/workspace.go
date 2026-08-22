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

// workspaceEntry mirrors workspacepkg.Workspace but also carries the stable
// registry slug (the map key), since Workspace.Name is only a display name
// and can differ from the slug that "workspace attach --workspace" and
// SPOOL_WORKSPACE actually accept.
type workspaceEntry struct {
	Slug workspacepkg.Name `json:"slug"`
	workspacepkg.Workspace
}

const maxWorkspaceIdentityAttempts = 8

// uniqueWorkspaceIdentity generates a workspace ID (and its derived state
// directory) that does not collide with any ID or state directory already
// present in registry. Workspace IDs are derived from only four random
// bytes, so a collision across enough workspaces is plausible; without this
// check two differently named workspaces could end up sharing one
// repos/<id> state directory and silently share graph state. Called while
// holding the registry lock (from within an UpdateRegistry mutation) so the
// collision check is against the authoritative, current registry contents.
func uniqueWorkspaceIdentity(registry *workspacepkg.Registry, root string) (workspacepkg.ID, string, error) {
	for attempt := 0; attempt < maxWorkspaceIdentityAttempts; attempt++ {
		id, err := workspacepkg.NewID()
		if err != nil {
			return "", "", err
		}
		stateDir, err := workspacepkg.RepositoryPath(root, id)
		if err != nil {
			return "", "", err
		}
		collision := false
		for _, other := range registry.Workspaces {
			if other.ID == id || other.StateDir == stateDir {
				collision = true
				break
			}
		}
		if !collision {
			return id, stateDir, nil
		}
	}
	return "", "", fmt.Errorf("generate workspace identity: could not find a unique ID after %d attempts", maxWorkspaceIdentityAttempts)
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
		Example:      "  spl workspace init ecommerce-platform\n  spl workspace use ecommerce-platform\n  spl workspace unset\n  spl workspace attach --workspace ecommerce-platform\n  spl workspace list\n  spl workspace current",
		SilenceUsage: true,
	}
	command.AddCommand(
		newWorkspaceInitCommand(registryRoot),
		newWorkspaceUseCommand(registryRoot),
		newWorkspaceUnsetCommand(registryRoot),
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
			var created workspacepkg.Workspace
			if err := workspacepkg.UpdateRegistry(root, func(registry *workspacepkg.Registry) error {
				if _, exists := registry.Workspaces[name]; exists {
					return fmt.Errorf("workspace %q already exists", name)
				}
				id, stateDir, err := uniqueWorkspaceIdentity(registry, root)
				if err != nil {
					return err
				}
				created = workspacepkg.Workspace{
					ID:        id,
					Name:      string(name),
					StateDir:  stateDir,
					CreatedAt: time.Now().UTC(),
					Paths:     nil,
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

func newWorkspaceUseCommand(registryRoot func() (string, error)) *cobra.Command {
	return &cobra.Command{
		Use:          "use <name>",
		Short:        "Set the active detached workspace",
		Long:         "Set the persisted active detached workspace preference for future sessions and write the selected workspace as JSON.",
		Example:      "  spl workspace use ecommerce-platform",
		Args:         cobra.ExactArgs(1),
		SilenceUsage: true,
		RunE: func(command *cobra.Command, args []string) error {
			name, err := workspacepkg.ParseName(args[0])
			if err != nil {
				return err
			}
			root, err := registryRoot()
			if err != nil {
				return err
			}
			if err := workspacepkg.SetCurrentWorkspace(root, name); err != nil {
				return err
			}
			registry, err := workspacepkg.LoadRegistry(root)
			if err != nil {
				return err
			}
			// SetCurrentWorkspace already confirmed name exists, but the
			// registry is reloaded here under a separate lock acquisition,
			// so a concurrent mutation (e.g. another process detaching or
			// otherwise rewriting the registry) could remove it in between.
			// Report that explicitly rather than silently encoding a
			// zero-value workspace.
			entry, exists := registry.Workspaces[name]
			if !exists {
				return fmt.Errorf("%w: workspace %q is not registered", workspacepkg.ErrWorkspaceNotRegistered, name)
			}
			if entry.Paths == nil {
				entry.Paths = []string{}
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(workspaceEntry{Slug: name, Workspace: entry})
		},
	}
}

// workspaceUnsetResult reports whether a persisted active-workspace
// preference existed before being cleared, so scripts can distinguish
// clearing an existing preference from a no-op on an already-clear one.
type workspaceUnsetResult struct {
	Cleared bool `json:"cleared"`
}

func newWorkspaceUnsetCommand(registryRoot func() (string, error)) *cobra.Command {
	return &cobra.Command{
		Use:          "unset",
		Short:        "Clear the active detached workspace preference",
		Long:         "Clear the persisted active detached workspace preference set by \"spl workspace use\" and write the result as JSON. Succeeds even when no preference is set, which also recovers from a stale preference that points at a workspace no longer in the registry.",
		Example:      "  spl workspace unset",
		Args:         cobra.NoArgs,
		SilenceUsage: true,
		RunE: func(command *cobra.Command, _ []string) error {
			root, err := registryRoot()
			if err != nil {
				return err
			}
			// A stale or malformed preference file must still be clearable:
			// "unset" is the documented recovery path when a broken
			// preference blocks every command, since bootstrap resolves the
			// state directory (honoring this preference) before any
			// subcommand -- including this one -- runs. So a best-effort
			// read failure is reported as if a preference was present
			// rather than surfacing a hard error that would defeat the
			// point of this recovery command.
			_, hadPreference, readErr := workspacepkg.CurrentWorkspaceName(root)
			if err := workspacepkg.ClearCurrentWorkspace(root); err != nil {
				return err
			}
			return json.NewEncoder(command.OutOrStdout()).Encode(workspaceUnsetResult{Cleared: hadPreference || readErr != nil})
		},
	}
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
			result := make([]workspaceEntry, 0, len(names))
			for _, name := range names {
				entry := registry.Workspaces[name]
				if entry.Paths == nil {
					entry.Paths = []string{}
				}
				result = append(result, workspaceEntry{Slug: name, Workspace: entry})
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
			return json.NewEncoder(command.OutOrStdout()).Encode(workspaceEntry{Slug: match.Name, Workspace: match.Workspace})
		},
	}
}

func detachWorkspacePath(root, path string) (workspacePathResult, error) {
	// Use CanonicalStoredPath (not CanonicalPath) so that detaching a path
	// whose repository was already deleted or moved still works: the
	// registry tolerates and stores such stale attachments (see
	// CanonicalStoredPath's documentation), and detach is the only way to
	// remove one, so it must not fail with ENOENT on the very entry it is
	// meant to clean up.
	canonicalPath, err := workspacepkg.CanonicalStoredPath(path)
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
