// Package repository provides durable graph storage and repository lifecycle operations.
package repository

import (
	"github.com/autonomous-bits/spool/internal/workspace"
)

const CurrentWorkspaceManifestVersion = workspace.CurrentManifestVersion

type (
	WorkspaceID       = workspace.ID
	WorkspaceName     = workspace.Name
	Workspace         = workspace.Workspace
	WorkspaceRegistry = workspace.Registry
)

// Workspace sentinel errors re-exported from the workspace package.
var (
	ErrWorkspaceNotRegistered          = workspace.ErrWorkspaceNotRegistered
	ErrWorkspaceExists                 = workspace.ErrWorkspaceExists
	ErrWorkspaceInvalidName            = workspace.ErrInvalidName
	ErrWorkspaceInvalidID              = workspace.ErrInvalidID
	ErrWorkspaceInvalidStorageRoot     = workspace.ErrInvalidStorageRoot
	ErrWorkspaceStorageRootUnavailable = workspace.ErrStorageRootUnavailable
	ErrWorkspaceManifestNotFound       = workspace.ErrManifestNotFound
	ErrWorkspaceInvalidManifest        = workspace.ErrInvalidManifest
	ErrWorkspaceManifestConflict       = workspace.ErrManifestConflict
	ErrWorkspaceInvalidRepositoryID    = workspace.ErrInvalidRepositoryID
)

// WorkspaceStorageRoot returns the platform-specific data directory.
func WorkspaceStorageRoot() (string, error) {
	return workspace.StorageRoot()
}

func ParseWorkspaceName(value string) (WorkspaceName, error) {
	return workspace.ParseName(value)
}

func NewWorkspaceID() (WorkspaceID, error) {
	return workspace.NewID()
}

func RepositoryWorkspacePath(root string, id WorkspaceID) (string, error) {
	return workspace.RepositoryPath(root, id)
}

func LoadWorkspaceRegistry(root string) (WorkspaceRegistry, error) {
	return workspace.LoadRegistry(root)
}

func UpdateWorkspaceRegistry(root string, update func(*WorkspaceRegistry) error) error {
	return workspace.UpdateRegistry(root, update)
}

// FindWorkspaceByID resolves detached state from its immutable identity.
func FindWorkspaceByID(root string, id WorkspaceID) (string, error) {
	return workspace.FindWorkspaceByID(root, id)
}

// WorkspaceManifest declares a checkout's portable workspace binding.
type WorkspaceManifest = workspace.Manifest

// DiscoverWorkspaceManifest searches a directory and its ancestors for a manifest.
func DiscoverWorkspaceManifest(workingDirectory string) (string, WorkspaceManifest, bool, error) {
	return workspace.DiscoverManifest(workingDirectory)
}

// WriteWorkspaceManifest atomically writes a checkout-owned workspace manifest.
func WriteWorkspaceManifest(repositoryRoot string, manifest WorkspaceManifest) error {
	return workspace.WriteManifest(repositoryRoot, manifest)
}
