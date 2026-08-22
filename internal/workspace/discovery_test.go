package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFindWorkspace(t *testing.T) {
	t.Run("finds attachment from nested directory", func(t *testing.T) {
		root := t.TempDir()
		repositoryPath := makeDirectory(t, root, "repositories", "storefront")
		addWorkspace(t, root, "storefront", "ws_00000001", []string{repositoryPath})
		workingDirectory := makeDirectory(t, repositoryPath, "src", "components")

		match, err := FindWorkspace(root, workingDirectory)
		if err != nil {
			t.Fatalf("FindWorkspace: %v", err)
		}
		if match.Name != "storefront" {
			t.Fatalf("workspace name = %q, want %q", match.Name, "storefront")
		}
	})

	t.Run("uses longest overlapping attachment", func(t *testing.T) {
		root := t.TempDir()
		parentPath := makeDirectory(t, root, "repositories", "platform")
		childPath := makeDirectory(t, parentPath, "frontend")
		addWorkspace(t, root, "platform", "ws_00000002", []string{parentPath})
		addWorkspace(t, root, "frontend", "ws_00000003", []string{childPath})
		workingDirectory := makeDirectory(t, childPath, "src")

		match, err := FindWorkspace(root, workingDirectory)
		if err != nil {
			t.Fatalf("FindWorkspace: %v", err)
		}
		if match.Name != "frontend" {
			t.Fatalf("workspace name = %q, want longest-match workspace %q", match.Name, "frontend")
		}
	})

	t.Run("does not match sibling path with a shared string prefix", func(t *testing.T) {
		root := t.TempDir()
		repositoryPath := makeDirectory(t, root, "repositories", "repo")
		siblingPath := makeDirectory(t, root, "repositories", "repository")
		addWorkspace(t, root, "repo", "ws_00000004", []string{repositoryPath})

		_, err := FindWorkspace(root, siblingPath)
		if !errors.Is(err, ErrWorkspaceNotFound) {
			t.Fatalf("FindWorkspace error = %v, want ErrWorkspaceNotFound", err)
		}
	})

	t.Run("canonicalizes symlinked working directory", func(t *testing.T) {
		root := t.TempDir()
		repositoryPath := makeDirectory(t, root, "repositories", "storefront")
		symlinkPath := filepath.Join(root, "links", "storefront")
		if err := os.MkdirAll(filepath.Dir(symlinkPath), 0o755); err != nil {
			t.Fatalf("create symlink parent: %v", err)
		}
		if err := os.Symlink(repositoryPath, symlinkPath); err != nil {
			t.Fatalf("create symlink: %v", err)
		}
		addWorkspace(t, root, "storefront", "ws_00000005", []string{repositoryPath})
		makeDirectory(t, repositoryPath, "src")

		match, err := FindWorkspace(root, filepath.Join(symlinkPath, "src"))
		if err != nil {
			t.Fatalf("FindWorkspace: %v", err)
		}
		if match.Name != "storefront" {
			t.Fatalf("workspace name = %q, want %q", match.Name, "storefront")
		}
	})

	t.Run("returns explicit no-match error", func(t *testing.T) {
		root := t.TempDir()
		repositoryPath := makeDirectory(t, root, "repositories", "storefront")
		otherPath := makeDirectory(t, root, "repositories", "payments")
		addWorkspace(t, root, "storefront", "ws_00000006", []string{repositoryPath})

		_, err := FindWorkspace(root, otherPath)
		if !errors.Is(err, ErrWorkspaceNotFound) {
			t.Fatalf("FindWorkspace error = %v, want ErrWorkspaceNotFound", err)
		}
	})
}

func TestFindWorkspaceDoesNotSkipInvalidAttachedPath(t *testing.T) {
	root := t.TempDir()
	registryPath, err := RegistryPath(root)
	if err != nil {
		t.Fatalf("RegistryPath: %v", err)
	}
	if err := os.WriteFile(registryPath, []byte(`
version = 1

[workspaces.storefront]
id = "ws_00000007"
name = "Storefront"
state_dir = "`+filepath.Join(root, "repos", "ws_00000007")+`"
created_at = "2026-08-22T15:00:00Z"
paths = ["missing-repository"]
`), 0o600); err != nil {
		t.Fatalf("write registry: %v", err)
	}

	_, err = FindWorkspace(root, root)
	if err == nil {
		t.Fatal("FindWorkspace unexpectedly ignored invalid attached path")
	}
	if errors.Is(err, ErrWorkspaceNotFound) {
		t.Fatalf("FindWorkspace error = %v, must not be ErrWorkspaceNotFound", err)
	}
}

func addWorkspace(t *testing.T, root string, name Name, id string, paths []string) {
	t.Helper()
	if err := UpdateRegistry(root, func(registry *Registry) error {
		registry.Workspaces[name] = testWorkspace(root, id, paths)
		return nil
	}); err != nil {
		t.Fatalf("add workspace %q: %v", name, err)
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
