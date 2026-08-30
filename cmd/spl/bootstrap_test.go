package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository"
)

func TestRepositoryStateDirFromArgs(t *testing.T) {
	lookupEnv := func(values map[string]string) func(string) (string, bool) {
		return func(key string) (string, bool) {
			value, ok := values[key]
			return value, ok
		}
	}

	t.Run("state-dir flag wins over environment", func(t *testing.T) {
		explicit := filepath.Join(t.TempDir(), "explicit")
		got, err := repositoryStateDirFromArgs([]string{"resolve", "--state-dir", explicit}, lookupEnv(map[string]string{envStateDir: "/unused"}), t.TempDir())
		if err != nil {
			t.Fatalf("repositoryStateDirFromArgs: %v", err)
		}
		if got != explicit {
			t.Fatalf("state directory = %q, want %q", got, explicit)
		}
	})

	t.Run("SPOOL_DIR overrides manifest discovery", func(t *testing.T) {
		checkout := t.TempDir()
		writeManifest(t, checkout, "ws_00000001")
		explicit := filepath.Join(t.TempDir(), "explicit")
		got, err := repositoryStateDirFromArgs(nil, lookupEnv(map[string]string{envStateDir: explicit}), checkout)
		if err != nil {
			t.Fatalf("repositoryStateDirFromArgs: %v", err)
		}
		if got != explicit {
			t.Fatalf("state directory = %q, want %q", got, explicit)
		}
	})

	t.Run("legacy SPOOL_WORKSPACE is ignored", func(t *testing.T) {
		workingDirectory := t.TempDir()
		got, err := repositoryStateDirFromArgs(nil, lookupEnv(map[string]string{"SPOOL_WORKSPACE": "legacy"}), workingDirectory)
		if err != nil {
			t.Fatalf("repositoryStateDirFromArgs: %v", err)
		}
		want := filepath.Join(workingDirectory, ".spl")
		if got != want {
			t.Fatalf("state directory = %q, want local fallback %q", got, want)
		}
	})
}

func TestRepositoryStateDirFrom(t *testing.T) {
	t.Run("manifest resolves from ancestor", func(t *testing.T) {
		root := configureStorageRoot(t)
		checkout := t.TempDir()
		stateDir := filepath.Join(root, "repos", "ws_00000002")
		writeRegistry(t, root, "ws_00000002", stateDir)
		writeManifest(t, checkout, "ws_00000002")

		got, err := repositoryStateDirFrom(filepath.Join(checkout, "nested", "directory"))
		if err != nil {
			t.Fatalf("repositoryStateDirFrom: %v", err)
		}
		if got != stateDir {
			t.Fatalf("state directory = %q, want %q", got, stateDir)
		}
	})

	t.Run("missing manifest workspace fails explicitly", func(t *testing.T) {
		checkout := t.TempDir()
		writeManifest(t, checkout, "ws_00000003")
		if _, err := repositoryStateDirFrom(checkout); err == nil {
			t.Fatal("repositoryStateDirFrom error = nil, want manifest lookup error")
		}
	})

	t.Run("malformed manifest fails explicitly", func(t *testing.T) {
		checkout := t.TempDir()
		if err := os.Mkdir(filepath.Join(checkout, ".spl"), 0o755); err != nil {
			t.Fatalf("create manifest directory: %v", err)
		}
		if err := os.WriteFile(filepath.Join(checkout, ".spl", "config.toml"), []byte("workspace_id = 'ws_bad'"), 0o600); err != nil {
			t.Fatalf("write manifest: %v", err)
		}
		if _, err := repositoryStateDirFrom(checkout); err == nil {
			t.Fatal("repositoryStateDirFrom error = nil, want malformed manifest error")
		}
	})

	t.Run("local repository remains the fallback", func(t *testing.T) {
		workingDirectory := t.TempDir()
		got, err := repositoryStateDirFrom(workingDirectory)
		if err != nil {
			t.Fatalf("repositoryStateDirFrom: %v", err)
		}
		want := filepath.Join(workingDirectory, ".spl")
		if got != want {
			t.Fatalf("state directory = %q, want %q", got, want)
		}
	})
}

func configureStorageRoot(t *testing.T) string {
	t.Helper()
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	root, err := repository.WorkspaceStorageRoot()
	if err != nil {
		t.Fatalf("WorkspaceStorageRoot: %v", err)
	}
	return root
}

func writeManifest(t *testing.T, checkout, id string) {
	t.Helper()
	if err := repository.WriteWorkspaceManifest(checkout, repository.WorkspaceManifest{
		FormatVersion: repository.CurrentWorkspaceManifestVersion,
		RepositoryID:  "github.com/acme/storefront",
		WorkspaceID:   repository.WorkspaceID(id),
	}); err != nil {
		t.Fatalf("WriteWorkspaceManifest: %v", err)
	}
}

func writeRegistry(t *testing.T, root, id, stateDir string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("create registry directory: %v", err)
	}
	contents := "version = 1\n\n[workspaces.storefront]\nid = \"" + id + "\"\nstate_dir = \"" + stateDir + "\"\n"
	if err := os.WriteFile(filepath.Join(root, "registry.toml"), []byte(contents), 0o600); err != nil {
		t.Fatalf("write registry: %v", err)
	}
}
