package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/autonomous-bits/spool/internal/repository"
)

func TestRepositoryStateDirFromArgs(t *testing.T) {
	lookupEnv := func(values map[string]string) func(string) (string, bool) {
		return func(key string) (string, bool) {
			value, ok := values[key]
			return value, ok
		}
	}

	t.Run("state-dir flag wins over everything", func(t *testing.T) {
		repositoryPath := makeDirectory(t, t.TempDir(), "repo")
		registeredStateDir := filepath.Join(t.TempDir(), "repos", "ws_00000010")
		registerWorkspace(t, "storefront", "ws_00000010", registeredStateDir, repositoryPath)
		explicitStateDir := filepath.Join(t.TempDir(), "explicit")

		resolvedStateDir, err := repositoryStateDirFromArgs(
			[]string{"resolve", "--state-dir", explicitStateDir},
			lookupEnv(map[string]string{"SPOOL_DIR": "/should/not/be/used"}),
			repositoryPath,
		)
		if err != nil {
			t.Fatalf("repositoryStateDirFromArgs: %v", err)
		}
		if resolvedStateDir != explicitStateDir {
			t.Fatalf("state directory = %q, want explicit %q", resolvedStateDir, explicitStateDir)
		}
	})

	t.Run("state-dir flag accepts equals syntax", func(t *testing.T) {
		explicitStateDir := filepath.Join(t.TempDir(), "explicit")
		resolvedStateDir, err := repositoryStateDirFromArgs(
			[]string{"--state-dir=" + explicitStateDir},
			lookupEnv(nil),
			t.TempDir(),
		)
		if err != nil {
			t.Fatalf("repositoryStateDirFromArgs: %v", err)
		}
		if resolvedStateDir != explicitStateDir {
			t.Fatalf("state directory = %q, want explicit %q", resolvedStateDir, explicitStateDir)
		}
	})

	t.Run("state-dir flag rejects empty value", func(t *testing.T) {
		_, err := repositoryStateDirFromArgs(
			[]string{"--state-dir", ""},
			lookupEnv(nil),
			t.TempDir(),
		)
		if err == nil {
			t.Fatal("repositoryStateDirFromArgs: want error for empty --state-dir value")
		}
	})

	t.Run("state-dir after -- argument terminator is treated as positional, not a flag", func(t *testing.T) {
		repositoryPath := makeDirectory(t, t.TempDir(), "repo")
		registeredStateDir := filepath.Join(t.TempDir(), "repos", "ws_00000015")
		registerWorkspace(t, "storefront", "ws_00000015", registeredStateDir, repositoryPath)

		resolvedStateDir, err := repositoryStateDirFromArgs(
			[]string{"commit", "-m", "--", "--state-dir", "message that looks like a flag"},
			lookupEnv(nil),
			repositoryPath,
		)
		if err != nil {
			t.Fatalf("repositoryStateDirFromArgs: %v", err)
		}
		if resolvedStateDir != registeredStateDir {
			t.Fatalf("state directory = %q, want registry-resolved %q (a --state-dir token after -- must not be treated as the flag)", resolvedStateDir, registeredStateDir)
		}
	})

	t.Run("SPOOL_DIR overrides registry and local fallback", func(t *testing.T) {
		repositoryPath := makeDirectory(t, t.TempDir(), "repo")
		registeredStateDir := filepath.Join(t.TempDir(), "repos", "ws_00000011")
		registerWorkspace(t, "storefront", "ws_00000011", registeredStateDir, repositoryPath)
		envStateDirValue := filepath.Join(t.TempDir(), "env-dir")

		resolvedStateDir, err := repositoryStateDirFromArgs(
			nil,
			lookupEnv(map[string]string{"SPOOL_DIR": envStateDirValue}),
			repositoryPath,
		)
		if err != nil {
			t.Fatalf("repositoryStateDirFromArgs: %v", err)
		}
		if resolvedStateDir != envStateDirValue {
			t.Fatalf("state directory = %q, want SPOOL_DIR value %q", resolvedStateDir, envStateDirValue)
		}
	})

	t.Run("SPOOL_WORKSPACE resolves a registered workspace by name", func(t *testing.T) {
		root := configureStorageRoot(t)
		unrelatedPath := makeDirectory(t, t.TempDir(), "other-repo")
		stateDir := filepath.Join(t.TempDir(), "repos", "ws_00000012")
		registerWorkspaceIn(t, root, "analytics", "ws_00000012", stateDir, unrelatedPath)

		resolvedStateDir, err := repositoryStateDirFromArgs(
			nil,
			lookupEnv(map[string]string{"SPOOL_WORKSPACE": "analytics"}),
			t.TempDir(),
		)
		if err != nil {
			t.Fatalf("repositoryStateDirFromArgs: %v", err)
		}
		if resolvedStateDir != stateDir {
			t.Fatalf("state directory = %q, want SPOOL_WORKSPACE-resolved %q", resolvedStateDir, stateDir)
		}
	})

	t.Run("SPOOL_WORKSPACE errors on unknown workspace name", func(t *testing.T) {
		configureStorageRoot(t)
		_, err := repositoryStateDirFromArgs(
			nil,
			lookupEnv(map[string]string{"SPOOL_WORKSPACE": "does-not-exist"}),
			t.TempDir(),
		)
		if err == nil {
			t.Fatal("repositoryStateDirFromArgs: want error for unregistered SPOOL_WORKSPACE")
		}
	})

	t.Run("SPOOL_DIR takes priority over SPOOL_WORKSPACE", func(t *testing.T) {
		root := configureStorageRoot(t)
		workspaceStateDir := filepath.Join(t.TempDir(), "repos", "ws_00000013")
		registerWorkspaceIn(t, root, "analytics", "ws_00000013", workspaceStateDir, makeDirectory(t, t.TempDir(), "other-repo"))
		envStateDirValue := filepath.Join(t.TempDir(), "env-dir")

		resolvedStateDir, err := repositoryStateDirFromArgs(
			nil,
			lookupEnv(map[string]string{"SPOOL_DIR": envStateDirValue, "SPOOL_WORKSPACE": "analytics"}),
			t.TempDir(),
		)
		if err != nil {
			t.Fatalf("repositoryStateDirFromArgs: %v", err)
		}
		if resolvedStateDir != envStateDirValue {
			t.Fatalf("state directory = %q, want SPOOL_DIR value %q", resolvedStateDir, envStateDirValue)
		}
	})

	t.Run("persisted current workspace resolves when no flag or env override is present", func(t *testing.T) {
		root := configureStorageRoot(t)
		stateDir := filepath.Join(t.TempDir(), "repos", "ws_00000016")
		registerWorkspaceIn(t, root, "analytics", "ws_00000016", stateDir, makeDirectory(t, t.TempDir(), "other-repo"))
		setCurrentWorkspace(t, root, "analytics")

		resolvedStateDir, err := repositoryStateDirFromArgs(nil, lookupEnv(nil), t.TempDir())
		if err != nil {
			t.Fatalf("repositoryStateDirFromArgs: %v", err)
		}
		if resolvedStateDir != stateDir {
			t.Fatalf("state directory = %q, want persisted current workspace state directory %q", resolvedStateDir, stateDir)
		}
	})

	t.Run("state-dir flag still wins over persisted current workspace preference", func(t *testing.T) {
		root := configureStorageRoot(t)
		preferredStateDir := filepath.Join(t.TempDir(), "repos", "ws_00000017")
		registerWorkspaceIn(t, root, "analytics", "ws_00000017", preferredStateDir, makeDirectory(t, t.TempDir(), "other-repo"))
		setCurrentWorkspace(t, root, "analytics")
		explicitStateDir := filepath.Join(t.TempDir(), "explicit")

		resolvedStateDir, err := repositoryStateDirFromArgs(
			[]string{"resolve", "--state-dir", explicitStateDir},
			lookupEnv(nil),
			t.TempDir(),
		)
		if err != nil {
			t.Fatalf("repositoryStateDirFromArgs: %v", err)
		}
		if resolvedStateDir != explicitStateDir {
			t.Fatalf("state directory = %q, want explicit %q", resolvedStateDir, explicitStateDir)
		}
	})

	t.Run("SPOOL_DIR still wins over persisted current workspace preference", func(t *testing.T) {
		root := configureStorageRoot(t)
		preferredStateDir := filepath.Join(t.TempDir(), "repos", "ws_00000018")
		registerWorkspaceIn(t, root, "analytics", "ws_00000018", preferredStateDir, makeDirectory(t, t.TempDir(), "other-repo"))
		setCurrentWorkspace(t, root, "analytics")
		envStateDirValue := filepath.Join(t.TempDir(), "env-dir")

		resolvedStateDir, err := repositoryStateDirFromArgs(
			nil,
			lookupEnv(map[string]string{"SPOOL_DIR": envStateDirValue}),
			t.TempDir(),
		)
		if err != nil {
			t.Fatalf("repositoryStateDirFromArgs: %v", err)
		}
		if resolvedStateDir != envStateDirValue {
			t.Fatalf("state directory = %q, want SPOOL_DIR value %q", resolvedStateDir, envStateDirValue)
		}
	})

	t.Run("SPOOL_WORKSPACE still wins over persisted current workspace preference", func(t *testing.T) {
		root := configureStorageRoot(t)
		persistedStateDir := filepath.Join(t.TempDir(), "repos", "ws_00000019")
		envWorkspaceStateDir := filepath.Join(t.TempDir(), "repos", "ws_00000020")
		registerWorkspaceIn(t, root, "analytics", "ws_00000019", persistedStateDir, makeDirectory(t, t.TempDir(), "analytics-repo"))
		registerWorkspaceIn(t, root, "billing", "ws_00000020", envWorkspaceStateDir, makeDirectory(t, t.TempDir(), "billing-repo"))
		setCurrentWorkspace(t, root, "analytics")

		resolvedStateDir, err := repositoryStateDirFromArgs(
			nil,
			lookupEnv(map[string]string{"SPOOL_WORKSPACE": "billing"}),
			t.TempDir(),
		)
		if err != nil {
			t.Fatalf("repositoryStateDirFromArgs: %v", err)
		}
		if resolvedStateDir != envWorkspaceStateDir {
			t.Fatalf("state directory = %q, want SPOOL_WORKSPACE-resolved %q", resolvedStateDir, envWorkspaceStateDir)
		}
	})

	t.Run("no override falls back to registry-based discovery", func(t *testing.T) {
		repositoryPath := makeDirectory(t, t.TempDir(), "repo")
		registeredStateDir := filepath.Join(t.TempDir(), "repos", "ws_00000014")
		registerWorkspace(t, "storefront", "ws_00000014", registeredStateDir, repositoryPath)

		resolvedStateDir, err := repositoryStateDirFromArgs(nil, lookupEnv(nil), repositoryPath)
		if err != nil {
			t.Fatalf("repositoryStateDirFromArgs: %v", err)
		}
		if resolvedStateDir != registeredStateDir {
			t.Fatalf("state directory = %q, want registry-resolved %q", resolvedStateDir, registeredStateDir)
		}
	})

	t.Run("stale persisted current workspace preference falls through to path-prefix discovery", func(t *testing.T) {
		root := configureStorageRoot(t)
		stateDir := filepath.Join(t.TempDir(), "repos", "ws_00000021")
		registerWorkspaceIn(t, root, "analytics", "ws_00000021", stateDir, makeDirectory(t, t.TempDir(), "other-repo"))
		setCurrentWorkspace(t, root, "analytics")
		removeWorkspace(t, root, "analytics")

		// A stale preference must not hard-error: repositoryStateDir runs
		// unconditionally in main() before any subcommand is dispatched, so
		// erroring here would block even "spl workspace use"/"unset", the
		// only commands that could otherwise fix a broken preference.
		workingDirectory := makeDirectory(t, t.TempDir(), "unattached-repo")
		resolvedStateDir, err := repositoryStateDirFromArgs(nil, lookupEnv(nil), workingDirectory)
		if err != nil {
			t.Fatalf("repositoryStateDirFromArgs: %v", err)
		}
		wantStateDir := filepath.Join(workingDirectory, ".spl")
		if resolvedStateDir != wantStateDir {
			t.Fatalf("state directory = %q, want local fallback %q", resolvedStateDir, wantStateDir)
		}
	})
}

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

	t.Run("manifest match wins over legacy registry path", func(t *testing.T) {
		root := configureStorageRoot(t)
		checkout := makeDirectory(t, t.TempDir(), "repo")
		legacyStateDir := filepath.Join(t.TempDir(), "repos", "ws_00000030")
		manifestStateDir := filepath.Join(t.TempDir(), "repos", "ws_00000031")
		registerWorkspaceIn(t, root, "legacy", "ws_00000030", legacyStateDir, checkout)
		registerWorkspaceIn(t, root, "portable", "ws_00000031", manifestStateDir, makeDirectory(t, t.TempDir(), "other"))
		if err := repository.WriteWorkspaceManifest(checkout, repository.WorkspaceManifest{
			FormatVersion: 1,
			RepositoryID:  "github.com/acme/storefront",
			WorkspaceID:   "ws_00000031",
		}); err != nil {
			t.Fatalf("write workspace manifest: %v", err)
		}

		resolvedStateDir, err := repositoryStateDirFrom(filepath.Join(checkout, "nested"))
		if err != nil {
			t.Fatalf("repositoryStateDirFrom: %v", err)
		}
		if resolvedStateDir != manifestStateDir {
			t.Fatalf("state directory = %q, want manifest state directory %q", resolvedStateDir, manifestStateDir)
		}
	})

	t.Run("manifest with missing local workspace fails explicitly", func(t *testing.T) {
		configureStorageRoot(t)
		checkout := makeDirectory(t, t.TempDir(), "repo")
		if err := repository.WriteWorkspaceManifest(checkout, repository.WorkspaceManifest{
			FormatVersion: 1,
			RepositoryID:  "github.com/acme/storefront",
			WorkspaceID:   "ws_00000032",
		}); err != nil {
			t.Fatalf("write workspace manifest: %v", err)
		}
		if _, err := repositoryStateDirFrom(checkout); err == nil {
			t.Fatal("repositoryStateDirFrom error = nil, want missing manifest workspace error")
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
	root, err := repository.WorkspaceStorageRoot()
	if err != nil {
		t.Fatalf("StorageRoot: %v", err)
	}
	return root
}

func registerWorkspace(t *testing.T, slug, id, stateDir string, paths ...string) {
	t.Helper()
	root := configureStorageRoot(t)
	registerWorkspaceIn(t, root, slug, id, stateDir, paths...)
}

// registerWorkspaceIn registers a workspace in the registry rooted at root,
// for tests that need to reuse a single storage root across multiple helper
// calls (configureStorageRoot always provisions a fresh temporary root).
func registerWorkspaceIn(t *testing.T, root, slug, id, stateDir string, paths ...string) {
	t.Helper()
	name, err := repository.ParseWorkspaceName(slug)
	if err != nil {
		t.Fatalf("ParseName(%q): %v", slug, err)
	}

	canonicalPaths := make([]string, 0, len(paths))
	for _, path := range paths {
		canonicalPath, err := repository.CanonicalWorkspacePath(path)
		if err != nil {
			t.Fatalf("CanonicalPath(%q): %v", path, err)
		}
		canonicalPaths = append(canonicalPaths, canonicalPath)
	}

	if err := repository.UpdateWorkspaceRegistry(root, func(registry *repository.WorkspaceRegistry) error {
		registry.Workspaces[name] = repository.Workspace{
			ID:        repository.WorkspaceID(id),
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

func setCurrentWorkspace(t *testing.T, root, slug string) {
	t.Helper()
	name, err := repository.ParseWorkspaceName(slug)
	if err != nil {
		t.Fatalf("ParseName(%q): %v", slug, err)
	}
	if err := repository.SetCurrentWorkspace(root, name); err != nil {
		t.Fatalf("SetCurrentWorkspace(%q): %v", slug, err)
	}
}

func removeWorkspace(t *testing.T, root, slug string) {
	t.Helper()
	name, err := repository.ParseWorkspaceName(slug)
	if err != nil {
		t.Fatalf("ParseName(%q): %v", slug, err)
	}
	if err := repository.UpdateWorkspaceRegistry(root, func(registry *repository.WorkspaceRegistry) error {
		delete(registry.Workspaces, name)
		return nil
	}); err != nil {
		t.Fatalf("UpdateRegistry(remove %q): %v", slug, err)
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
