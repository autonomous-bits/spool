package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/autonomous-bits/spool/internal/workspace"
)

func TestRepositoryStateDirFrom(t *testing.T) {
	t.Run("registry match wins over local spl directory", func(t *testing.T) {
		repositoryPath := makeDirectory(t, t.TempDir(), "repo")
		localStateDir := makeDirectory(t, repositoryPath, ".spl")
		stateDir := filepath.Join(t.TempDir(), "repos", "ws_00000001")

		registerWorkspace(t, "storefront", "ws_00000001", stateDir, repositoryPath)

		resolvedStateDir, err := repositoryStateDirFrom(repositoryPath)
		if err != nil {
			t.Fatalf("repositoryStateDirFrom: %v", err)
		}
		if resolvedStateDir != stateDir {
			t.Fatalf("state directory = %q, want registered workspace state directory %q instead of local %q", resolvedStateDir, stateDir, localStateDir)
		}
	})

	t.Run("no registry match falls back to local spl directory", func(t *testing.T) {
		configureStorageRoot(t)
		workingDirectory := makeDirectory(t, t.TempDir(), "repo")
		localStateDir := makeDirectory(t, workingDirectory, ".spl")

		resolvedStateDir, err := repositoryStateDirFrom(workingDirectory)
		if err != nil {
			t.Fatalf("repositoryStateDirFrom: %v", err)
		}
		if resolvedStateDir != localStateDir {
			t.Fatalf("state directory = %q, want local state directory %q", resolvedStateDir, localStateDir)
		}
	})

	t.Run("no registry or local state but go work marks repository root", func(t *testing.T) {
		configureStorageRoot(t)
		workingDirectory := t.TempDir()
		if err := os.WriteFile(filepath.Join(workingDirectory, "go.work"), []byte("go 1.26\n"), 0o600); err != nil {
			t.Fatalf("write go.work: %v", err)
		}

		resolvedStateDir, err := repositoryStateDirFrom(workingDirectory)
		if err != nil {
			t.Fatalf("repositoryStateDirFrom: %v", err)
		}
		want := filepath.Join(workingDirectory, ".spl")
		if resolvedStateDir != want {
			t.Fatalf("state directory = %q, want %q", resolvedStateDir, want)
		}
	})

	t.Run("no registry or repository markers falls back to starting directory", func(t *testing.T) {
		configureStorageRoot(t)
		startDirectory := makeDirectory(t, t.TempDir(), "repo", "nested")

		resolvedStateDir, err := repositoryStateDirFrom(startDirectory)
		if err != nil {
			t.Fatalf("repositoryStateDirFrom: %v", err)
		}
		want := filepath.Join(startDirectory, ".spl")
		if resolvedStateDir != want {
			t.Fatalf("state directory = %q, want %q", resolvedStateDir, want)
		}
	})

	t.Run("unrelated registry entry does not prevent local fallback", func(t *testing.T) {
		workingDirectory := makeDirectory(t, t.TempDir(), "repo")
		localStateDir := makeDirectory(t, workingDirectory, ".spl")
		unrelatedPath := makeDirectory(t, t.TempDir(), "other-repo")
		stateDir := filepath.Join(t.TempDir(), "repos", "ws_00000002")

		registerWorkspace(t, "analytics", "ws_00000002", stateDir, unrelatedPath)

		resolvedStateDir, err := repositoryStateDirFrom(workingDirectory)
		if err != nil {
			t.Fatalf("repositoryStateDirFrom: %v", err)
		}
		if resolvedStateDir != localStateDir {
			t.Fatalf("state directory = %q, want local fallback %q", resolvedStateDir, localStateDir)
		}
	})

	t.Run("nested working directory resolves to attached workspace state directory", func(t *testing.T) {
		repositoryPath := makeDirectory(t, t.TempDir(), "repo")
		workingDirectory := makeDirectory(t, repositoryPath, "sub", "deeper")
		stateDir := filepath.Join(t.TempDir(), "repos", "ws_00000003")

		registerWorkspace(t, "platform", "ws_00000003", stateDir, repositoryPath)

		resolvedStateDir, err := repositoryStateDirFrom(workingDirectory)
		if err != nil {
			t.Fatalf("repositoryStateDirFrom: %v", err)
		}
		if resolvedStateDir != stateDir {
			t.Fatalf("state directory = %q, want registered workspace state directory %q", resolvedStateDir, stateDir)
		}
	})
}

func configureStorageRoot(t *testing.T) string {
	t.Helper()
	xdgDataHome := t.TempDir()
	t.Setenv("XDG_DATA_HOME", xdgDataHome)
	root, err := workspace.StorageRoot()
	if err != nil {
		t.Fatalf("StorageRoot: %v", err)
	}
	return root
}

func registerWorkspace(t *testing.T, slug, id, stateDir string, paths ...string) {
	t.Helper()
	root := configureStorageRoot(t)
	name, err := workspace.ParseName(slug)
	if err != nil {
		t.Fatalf("ParseName(%q): %v", slug, err)
	}

	canonicalPaths := make([]string, 0, len(paths))
	for _, path := range paths {
		canonicalPath, err := workspace.CanonicalPath(path)
		if err != nil {
			t.Fatalf("CanonicalPath(%q): %v", path, err)
		}
		canonicalPaths = append(canonicalPaths, canonicalPath)
	}

	if err := workspace.UpdateRegistry(root, func(registry *workspace.Registry) error {
		registry.Workspaces[name] = workspace.Workspace{
			ID:        workspace.ID(id),
			Name:      "Workspace " + id,
			StateDir:  stateDir,
			CreatedAt: time.Date(2026, time.August, 22, 15, 0, 0, 0, time.UTC),
			Paths:     canonicalPaths,
		}
		return nil
	}); err != nil {
		t.Fatalf("UpdateRegistry: %v", err)
	}
}

func makeDirectory(t *testing.T, elements ...string) string {
	t.Helper()
	path := filepath.Join(elements...)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("create directory %q: %v", path, err)
	}
	return path
}
