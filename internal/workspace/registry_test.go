package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/flock"
)

func TestRegistryTOMLRoundTripAndPersistence(t *testing.T) {
	root := t.TempDir()
	stateDirectory := filepath.Join(root, "repos", "ws_8f1e2a3b")
	repositoryPath := filepath.Join(root, "repositories", "storefront")
	if err := os.MkdirAll(repositoryPath, 0o755); err != nil {
		t.Fatalf("create repository path: %v", err)
	}
	canonicalRepositoryPath, err := CanonicalPath(repositoryPath)
	if err != nil {
		t.Fatalf("CanonicalPath: %v", err)
	}
	createdAt := time.Date(2026, time.August, 22, 15, 0, 0, 0, time.UTC)

	if err := UpdateRegistry(root, func(registry *Registry) error {
		registry.Workspaces[Name("ecommerce-platform")] = Workspace{
			ID:        "ws_8f1e2a3b",
			Name:      "E-Commerce Platform",
			StateDir:  stateDirectory,
			CreatedAt: createdAt,
			Paths:     []string{repositoryPath},
		}
		return nil
	}); err != nil {
		t.Fatalf("UpdateRegistry: %v", err)
	}

	registryPath, err := RegistryPath(root)
	if err != nil {
		t.Fatalf("RegistryPath: %v", err)
	}
	contents, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatalf("read registry: %v", err)
	}
	if !strings.Contains(string(contents), "version = 1") || !strings.Contains(string(contents), "[workspaces.ecommerce-platform]") {
		t.Fatalf("registry is not persisted as expected TOML:\n%s", contents)
	}

	registry, err := LoadRegistry(root)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	workspace, ok := registry.Workspaces["ecommerce-platform"]
	if !ok {
		t.Fatal("persisted workspace is missing")
	}
	if workspace.ID != "ws_8f1e2a3b" || workspace.Name != "E-Commerce Platform" || workspace.StateDir != stateDirectory {
		t.Fatalf("workspace = %#v, want documented fields", workspace)
	}
	if !workspace.CreatedAt.Equal(createdAt) {
		t.Fatalf("created at = %s, want %s", workspace.CreatedAt, createdAt)
	}
	if len(workspace.Paths) != 1 || workspace.Paths[0] != canonicalRepositoryPath {
		t.Fatalf("paths = %#v, want [%q]", workspace.Paths, canonicalRepositoryPath)
	}
}

func TestUpdateRegistryCanonicalizesSymlinkedRepositoryPath(t *testing.T) {
	root := t.TempDir()
	repositoryPath := filepath.Join(root, "repositories", "storefront")
	symlinkPath := filepath.Join(root, "links", "storefront")
	if err := os.MkdirAll(repositoryPath, 0o755); err != nil {
		t.Fatalf("create repository path: %v", err)
	}
	canonicalRepositoryPath, err := CanonicalPath(repositoryPath)
	if err != nil {
		t.Fatalf("CanonicalPath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(symlinkPath), 0o755); err != nil {
		t.Fatalf("create symlink parent: %v", err)
	}
	if err := os.Symlink(repositoryPath, symlinkPath); err != nil {
		t.Fatalf("create symlink: %v", err)
	}

	if err := UpdateRegistry(root, func(registry *Registry) error {
		registry.Workspaces[Name("ecommerce-platform")] = testWorkspace(root, "ws_8f1e2a3b", []string{filepath.Join(symlinkPath, ".")})
		return nil
	}); err != nil {
		t.Fatalf("UpdateRegistry: %v", err)
	}

	registry, err := LoadRegistry(root)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	paths := registry.Workspaces["ecommerce-platform"].Paths
	if len(paths) != 1 || paths[0] != canonicalRepositoryPath {
		t.Fatalf("canonical paths = %#v, want [%q]", paths, canonicalRepositoryPath)
	}
}

func TestAttachPathRejectsWorkspaceCollision(t *testing.T) {
	root := t.TempDir()
	repositoryPath := filepath.Join(root, "repositories", "storefront")
	symlinkPath := filepath.Join(root, "linked-storefront")
	if err := os.MkdirAll(repositoryPath, 0o755); err != nil {
		t.Fatalf("create repository path: %v", err)
	}
	if err := os.Symlink(repositoryPath, symlinkPath); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	if err := UpdateRegistry(root, func(registry *Registry) error {
		registry.Workspaces[Name("ecommerce-platform")] = testWorkspace(root, "ws_8f1e2a3b", nil)
		registry.Workspaces[Name("analytics-pipeline")] = testWorkspace(root, "ws_9c4d1e2f", nil)
		return nil
	}); err != nil {
		t.Fatalf("create workspaces: %v", err)
	}
	if err := AttachPath(root, "ecommerce-platform", repositoryPath); err != nil {
		t.Fatalf("first AttachPath: %v", err)
	}
	if err := AttachPath(root, "analytics-pipeline", symlinkPath); !errors.Is(err, ErrPathAlreadyAttached) {
		t.Fatalf("second AttachPath error = %v, want ErrPathAlreadyAttached", err)
	}
}

func TestLoadRegistryRejectsMalformedTOML(t *testing.T) {
	root := t.TempDir()
	registryPath, err := RegistryPath(root)
	if err != nil {
		t.Fatalf("RegistryPath: %v", err)
	}
	if err := os.WriteFile(registryPath, []byte("version = ["), 0o600); err != nil {
		t.Fatalf("write malformed registry: %v", err)
	}
	if _, err := LoadRegistry(root); err == nil || !strings.Contains(err.Error(), "decode workspace registry") {
		t.Fatalf("LoadRegistry error = %v, want malformed TOML decode error", err)
	}
}

func TestLoadRegistryWaitsForRegistryLock(t *testing.T) {
	root := t.TempDir()
	lockPath, err := RegistryLockPath(root)
	if err != nil {
		t.Fatalf("RegistryLockPath: %v", err)
	}
	lock := flock.New(lockPath)
	if err := lock.Lock(); err != nil {
		t.Fatalf("lock registry: %v", err)
	}
	locked := true
	defer func() {
		if locked {
			if err := lock.Unlock(); err != nil {
				t.Errorf("unlock registry: %v", err)
			}
		}
	}()

	loaded := make(chan error, 1)
	go func() {
		_, err := LoadRegistry(root)
		loaded <- err
	}()

	select {
	case err := <-loaded:
		t.Fatalf("LoadRegistry returned while registry.lock was held: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := lock.Unlock(); err != nil {
		t.Fatalf("unlock registry: %v", err)
	}
	locked = false
	select {
	case err := <-loaded:
		if err != nil {
			t.Fatalf("LoadRegistry after unlock: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("LoadRegistry did not continue after registry.lock was released")
	}
}

func testWorkspace(root, id string, paths []string) Workspace {
	return Workspace{
		ID:        ID(id),
		Name:      "Workspace " + id,
		StateDir:  filepath.Join(root, "repos", id),
		CreatedAt: time.Date(2026, time.August, 22, 15, 0, 0, 0, time.UTC),
		Paths:     paths,
	}
}
