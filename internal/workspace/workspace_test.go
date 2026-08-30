package workspace

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestIDValidation(t *testing.T) {
	for _, id := range []ID{"ws_8f1e2a3b", "ws_00000000", "ws_abcdef12"} {
		if err := id.Validate(); err != nil {
			t.Fatalf("Validate(%q): %v", id, err)
		}
	}
	for _, id := range []ID{"", "workspace-123", "ws_8f1e2a3", "ws_8f1e2a3bb", "ws_8f1e2a3g", "WS_8f1e2a3b"} {
		if err := id.Validate(); !errors.Is(err, ErrInvalidID) {
			t.Fatalf("Validate(%q) error = %v, want ErrInvalidID", id, err)
		}
	}
}

func TestStorageRootUsesXDGDataHome(t *testing.T) {
	xdgDataHome := filepath.Join(string(filepath.Separator), "workspace-test", "xdg-data")
	t.Setenv("HOME", "")
	t.Setenv("XDG_DATA_HOME", xdgDataHome)
	root, err := StorageRoot()
	if err != nil {
		t.Fatalf("StorageRoot: %v", err)
	}
	if root != filepath.Join(xdgDataHome, "spool") {
		t.Fatalf("StorageRoot = %q", root)
	}
}

func TestRepositoryPathIsDeterministicAndContainedByStorageRoot(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "workspace-test", "state", "spool")
	got, err := RepositoryPath(root, "ws_8f1e2a3b")
	if err != nil {
		t.Fatalf("RepositoryPath: %v", err)
	}
	want := filepath.Join(root, "repos", "ws_8f1e2a3b")
	if got != want {
		t.Fatalf("RepositoryPath = %q, want %q", got, want)
	}
}
