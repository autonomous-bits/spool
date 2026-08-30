// Package repository provides durable graph storage and repository lifecycle operations.
package repository

import (
	"github.com/autonomous-bits/spool/internal/workspace"
)

// Workspace domain types re-exported from the workspace package.
type (
	Workspace         = workspace.Workspace
	WorkspaceName     = workspace.Name
	WorkspaceID       = workspace.ID
	WorkspaceRegistry = workspace.Registry
	WorkspaceMatch    = workspace.WorkspaceMatch
)

// Workspace sentinel errors re-exported from the workspace package.
var (
	ErrWorkspaceNotFound                = workspace.ErrWorkspaceNotFound
	ErrWorkspaceNotRegistered           = workspace.ErrWorkspaceNotRegistered
	ErrWorkspacePathAlreadyAttached     = workspace.ErrPathAlreadyAttached
	ErrWorkspaceInvalidName             = workspace.ErrInvalidName
	ErrWorkspaceInvalidID               = workspace.ErrInvalidID
	ErrWorkspaceInvalid                 = workspace.ErrInvalidWorkspace
	ErrWorkspaceInvalidRegistry         = workspace.ErrInvalidRegistry
	ErrWorkspaceInvalidStorageRoot      = workspace.ErrInvalidStorageRoot
	ErrWorkspaceStorageRootUnavailable  = workspace.ErrStorageRootUnavailable
	ErrWorkspaceInvalidCurrentWorkspace = workspace.ErrInvalidCurrentWorkspace
)

// WorkspaceStorageRoot returns the platform-specific data directory.
func WorkspaceStorageRoot() (string, error) {
	return workspace.StorageRoot()
}

// WorkspaceRegistryPath returns the registry file path within root.
func WorkspaceRegistryPath(root string) (string, error) {
	return workspace.RegistryPath(root)
}

// ParseWorkspaceName validates and normalizes raw workspace names.
func ParseWorkspaceName(value string) (WorkspaceName, error) {
	return workspace.ParseName(value)
}

// NewWorkspaceID generates a new random hexadecimal workspace identity.
func NewWorkspaceID() (WorkspaceID, error) {
	return workspace.NewID()
}

// RepositoryWorkspacePath constructs the state directory path for a workspace.
func RepositoryWorkspacePath(root string, id WorkspaceID) (string, error) {
	return workspace.RepositoryPath(root, id)
}

// LoadWorkspaceRegistry reads and validates the workspace registry file.
func LoadWorkspaceRegistry(root string) (WorkspaceRegistry, error) {
	return workspace.LoadRegistry(root)
}

// UpdateWorkspaceRegistry executes a mutation closure under the workspace lock.
func UpdateWorkspaceRegistry(root string, update func(*WorkspaceRegistry) error) error {
	return workspace.UpdateRegistry(root, update)
}

// AttachWorkspacePath associates path with workspace name.
func AttachWorkspacePath(root string, name WorkspaceName, path string) error {
	return workspace.AttachPath(root, name, path)
}

// CurrentWorkspaceName returns the active workspace preference.
func CurrentWorkspaceName(root string) (WorkspaceName, bool, error) {
	return workspace.CurrentWorkspaceName(root)
}

// SetCurrentWorkspace persists the active workspace preference.
func SetCurrentWorkspace(root string, name WorkspaceName) error {
	return workspace.SetCurrentWorkspace(root, name)
}

// ClearCurrentWorkspace removes the active workspace preference file.
func ClearCurrentWorkspace(root string) error {
	return workspace.ClearCurrentWorkspace(root)
}

// FindWorkspace selects the workspace whose attached paths match targetDirectory.
func FindWorkspace(root, targetDirectory string) (WorkspaceMatch, error) {
	return workspace.FindWorkspace(root, targetDirectory)
}

// CanonicalWorkspacePath resolves target path to an absolute path.
func CanonicalWorkspacePath(path string) (string, error) {
	return workspace.CanonicalPath(path)
}

// CanonicalStoredWorkspacePath resolves target path for registry persistence.
func CanonicalStoredWorkspacePath(path string) (string, error) {
	return workspace.CanonicalStoredPath(path)
}
