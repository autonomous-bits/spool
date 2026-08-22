package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/flock"
	"github.com/pelletier/go-toml/v2"
)

func TestSetCurrentWorkspacePersistsAndCanBeReadBack(t *testing.T) {
	root := t.TempDir()
	addWorkspace(t, root, "storefront", "ws_00000008", nil)

	if err := SetCurrentWorkspace(root, "storefront"); err != nil {
		t.Fatalf("SetCurrentWorkspace: %v", err)
	}

	contents, err := os.ReadFile(filepath.Join(root, currentWorkspaceFilename))
	if err != nil {
		t.Fatalf("read current workspace preference: %v", err)
	}
	// Decode rather than string-match the raw file: go-toml/v2 currently
	// emits single-quoted literal strings for simple values (e.g.
	// name = 'storefront'), but asserting on that exact quoting style would
	// make this test brittle to unrelated encoder formatting changes.
	var persisted currentWorkspacePreference
	if err := toml.Unmarshal(contents, &persisted); err != nil {
		t.Fatalf("decode persisted current workspace preference:\n%s\nerror: %v", contents, err)
	}
	if persisted.Version != currentWorkspaceVersion {
		t.Fatalf("persisted version = %d, want %d:\n%s", persisted.Version, currentWorkspaceVersion, contents)
	}
	if persisted.Name != "storefront" {
		t.Fatalf("persisted name = %q, want %q:\n%s", persisted.Name, "storefront", contents)
	}

	name, ok, err := CurrentWorkspaceName(root)
	if err != nil {
		t.Fatalf("CurrentWorkspaceName: %v", err)
	}
	if !ok {
		t.Fatal("CurrentWorkspaceName reported no persisted preference")
	}
	if name != "storefront" {
		t.Fatalf("current workspace name = %q, want %q", name, "storefront")
	}
}

func TestSetCurrentWorkspaceRejectsUnregisteredWorkspace(t *testing.T) {
	root := t.TempDir()
	addWorkspace(t, root, "storefront", "ws_00000009", nil)

	err := SetCurrentWorkspace(root, "payments")
	if !errors.Is(err, ErrWorkspaceNotRegistered) {
		t.Fatalf("SetCurrentWorkspace error = %v, want ErrWorkspaceNotRegistered", err)
	}

	name, ok, err := CurrentWorkspaceName(root)
	if err != nil {
		t.Fatalf("CurrentWorkspaceName after failed set: %v", err)
	}
	if ok {
		t.Fatalf("current workspace name = %q, want no persisted preference", name)
	}
	if _, err := os.Stat(filepath.Join(root, currentWorkspaceFilename)); !os.IsNotExist(err) {
		t.Fatalf("current workspace preference file still exists after failed set: %v", err)
	}
}

func TestCurrentWorkspaceNameWithoutPreference(t *testing.T) {
	root := t.TempDir()

	name, ok, err := CurrentWorkspaceName(root)
	if err != nil {
		t.Fatalf("CurrentWorkspaceName: %v", err)
	}
	if ok {
		t.Fatalf("current workspace name = %q, want no persisted preference", name)
	}
}

func TestClearCurrentWorkspaceRemovesPreference(t *testing.T) {
	root := t.TempDir()
	addWorkspace(t, root, "storefront", "ws_0000000a", nil)
	if err := SetCurrentWorkspace(root, "storefront"); err != nil {
		t.Fatalf("SetCurrentWorkspace: %v", err)
	}

	if err := ClearCurrentWorkspace(root); err != nil {
		t.Fatalf("ClearCurrentWorkspace: %v", err)
	}

	name, ok, err := CurrentWorkspaceName(root)
	if err != nil {
		t.Fatalf("CurrentWorkspaceName after clear: %v", err)
	}
	if ok {
		t.Fatalf("current workspace name = %q, want no persisted preference", name)
	}
	if _, err := os.Stat(filepath.Join(root, currentWorkspaceFilename)); !os.IsNotExist(err) {
		t.Fatalf("current workspace preference file still exists after clear: %v", err)
	}
}

func TestClearCurrentWorkspaceWithoutPreferenceDoesNotError(t *testing.T) {
	root := t.TempDir()

	if err := ClearCurrentWorkspace(root); err != nil {
		t.Fatalf("ClearCurrentWorkspace: %v", err)
	}
}

func TestSetCurrentWorkspaceWaitsForRegistryLock(t *testing.T) {
	root := t.TempDir()
	addWorkspace(t, root, "storefront", "ws_0000000b", nil)

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

	updated := make(chan error, 1)
	go func() {
		updated <- SetCurrentWorkspace(root, "storefront")
	}()

	select {
	case err := <-updated:
		t.Fatalf("SetCurrentWorkspace returned while registry.lock was held: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := lock.Unlock(); err != nil {
		t.Fatalf("unlock registry: %v", err)
	}
	locked = false

	select {
	case err := <-updated:
		if err != nil {
			t.Fatalf("SetCurrentWorkspace after unlock: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("SetCurrentWorkspace did not continue after registry.lock was released")
	}

	name, ok, err := CurrentWorkspaceName(root)
	if err != nil {
		t.Fatalf("CurrentWorkspaceName: %v", err)
	}
	if !ok || name != "storefront" {
		t.Fatalf("current workspace = (%q, %t), want (%q, true)", name, ok, "storefront")
	}
}

func TestCurrentWorkspaceRejectsInvalidPreference(t *testing.T) {
	tests := []struct {
		name     string
		contents string
		wantErr  error
		wantText string
	}{
		{
			name:     "malformed toml",
			contents: "version = [",
			wantText: "decode current workspace preference",
		},
		{
			name: "invalid workspace name",
			contents: `
version = 1
name = "Not A Slug"
`,
			wantErr: ErrInvalidCurrentWorkspace,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, currentWorkspaceFilename), []byte(test.contents), 0o600); err != nil {
				t.Fatalf("write current workspace preference: %v", err)
			}

			_, _, err := CurrentWorkspaceName(root)
			if test.wantErr != nil && !errors.Is(err, test.wantErr) {
				t.Fatalf("CurrentWorkspaceName error = %v, want %v", err, test.wantErr)
			}
			if test.wantText != "" && (err == nil || !strings.Contains(err.Error(), test.wantText)) {
				t.Fatalf("CurrentWorkspaceName error = %v, want text %q", err, test.wantText)
			}
		})
	}
}

func TestCurrentWorkspaceRejectsRelativeRoot(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "set",
			run: func() error {
				return SetCurrentWorkspace("relative-root", "storefront")
			},
		},
		{
			name: "get",
			run: func() error {
				_, _, err := CurrentWorkspaceName("relative-root")
				return err
			},
		},
		{
			name: "clear",
			run: func() error {
				return ClearCurrentWorkspace("relative-root")
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.run(); !errors.Is(err, ErrInvalidStorageRoot) {
				t.Fatalf("%s error = %v, want ErrInvalidStorageRoot", test.name, err)
			}
		})
	}
}
