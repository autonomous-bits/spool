package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteAndDiscoverManifest(t *testing.T) {
	root := t.TempDir()
	checkout := filepath.Join(root, "checkout")
	if err := os.MkdirAll(filepath.Join(checkout, "cmd", "spl"), 0o755); err != nil {
		t.Fatalf("create checkout: %v", err)
	}
	manifest := Manifest{
		FormatVersion: CurrentManifestVersion,
		RepositoryID:  "github.com/acme/storefront",
		WorkspaceID:   "ws_8f1e2a3b",
	}

	if err := WriteManifest(checkout, manifest); err != nil {
		t.Fatalf("WriteManifest: %v", err)
	}
	foundRoot, found, ok, err := DiscoverManifest(filepath.Join(checkout, "cmd", "spl"))
	if err != nil {
		t.Fatalf("DiscoverManifest: %v", err)
	}
	if !ok || foundRoot != checkout || found != manifest {
		t.Fatalf("manifest discovery = (%q, %#v, %t), want (%q, %#v, true)", foundRoot, found, ok, checkout, manifest)
	}
}

func TestWriteManifestRejectsConflictAndLocalRepositoryControlState(t *testing.T) {
	root := t.TempDir()
	manifest := Manifest{FormatVersion: CurrentManifestVersion, RepositoryID: "github.com/acme/storefront", WorkspaceID: "ws_8f1e2a3b"}
	if err := WriteManifest(root, manifest); err != nil {
		t.Fatalf("initial WriteManifest: %v", err)
	}
	conflicting := manifest
	conflicting.WorkspaceID = "ws_00000000"
	if err := WriteManifest(root, conflicting); !errors.Is(err, ErrManifestConflict) {
		t.Fatalf("WriteManifest conflict error = %v, want ErrManifestConflict", err)
	}

	localRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(localRoot, ".spl"), 0o755); err != nil {
		t.Fatalf("create local control state: %v", err)
	}
	if err := os.WriteFile(filepath.Join(localRoot, ".spl", "config.toml"), []byte("format_version = 1\ndefault_branch = 'main'\n"), 0o600); err != nil {
		t.Fatalf("write local control state: %v", err)
	}
	if err := WriteManifest(localRoot, manifest); !errors.Is(err, ErrManifestConflict) {
		t.Fatalf("WriteManifest local control state error = %v, want ErrManifestConflict", err)
	}
}

func TestDiscoverManifestIgnoresLocalRepositoryConfigAndRejectsInvalidManifest(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".spl"), 0o755); err != nil {
		t.Fatalf("create state directory: %v", err)
	}

	path := filepath.Join(root, ".spl", "config.toml")
	if err := os.WriteFile(path, []byte("format_version = 1\ndefault_branch = 'main'\n"), 0o600); err != nil {
		t.Fatalf("write local config: %v", err)
	}
	if _, _, found, err := DiscoverManifest(root); err != nil || found {
		t.Fatalf("DiscoverManifest local config = (_, _, %t, %v), want (_, _, false, nil)", found, err)
	}
	if err := os.WriteFile(path, []byte("format_version = 1\nworkspace_id = 'ws_bad'\nrepository_id = '/host/path'\n"), 0o600); err != nil {
		t.Fatalf("write invalid manifest: %v", err)
	}
	if _, _, _, err := DiscoverManifest(root); !errors.Is(err, ErrInvalidManifest) {
		t.Fatalf("DiscoverManifest invalid error = %v, want ErrInvalidManifest", err)
	}
}

func TestFindWorkspaceByID(t *testing.T) {
	root := t.TempDir()
	if err := UpdateRegistry(root, func(registry *Registry) error {
		registry.Workspaces["storefront"] = testWorkspace(root, "ws_8f1e2a3b", nil)
		return nil
	}); err != nil {
		t.Fatalf("UpdateRegistry: %v", err)
	}
	match, err := FindWorkspaceByID(root, "ws_8f1e2a3b")
	if err != nil {
		t.Fatalf("FindWorkspaceByID: %v", err)
	}
	if match.Name != "storefront" {
		t.Fatalf("workspace name = %q, want storefront", match.Name)
	}
	if _, err := FindWorkspaceByID(root, "ws_00000000"); !errors.Is(err, ErrWorkspaceNotRegistered) {
		t.Fatalf("FindWorkspaceByID missing error = %v, want ErrWorkspaceNotRegistered", err)
	}
}
