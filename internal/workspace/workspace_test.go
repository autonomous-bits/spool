package workspace

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestParseName(t *testing.T) {
	t.Run("accepts lowercase slug", func(t *testing.T) {
		name, err := ParseName("ecommerce-platform")
		if err != nil {
			t.Fatalf("ParseName: %v", err)
		}
		if name != "ecommerce-platform" {
			t.Fatalf("name = %q, want %q", name, "ecommerce-platform")
		}
	})

	for _, value := range []string{
		"", "-leading", "trailing-", "two--hyphens", "Uppercase", "has space",
		"contains/slash", `contains\backslash`, ".", "..",
	} {
		t.Run(value, func(t *testing.T) {
			if _, err := ParseName(value); !errors.Is(err, ErrInvalidName) {
				t.Fatalf("ParseName(%q) error = %v, want ErrInvalidName", value, err)
			}
		})
	}
}

func TestIDValidation(t *testing.T) {
	for _, id := range []ID{"ws_8f1e2a3b", "ws_00000000", "ws_abcdef12"} {
		if err := id.Validate(); err != nil {
			t.Fatalf("Validate(%q): %v", id, err)
		}
	}
	for _, id := range []ID{
		"", "workspace-123", "ws_8f1e2a3", "ws_8f1e2a3bb", "ws_8f1e2a3g",
		"WS_8f1e2a3b", "ws-8f1e2a3b", "ws_8F1E2A3B",
	} {
		if err := id.Validate(); !errors.Is(err, ErrInvalidID) {
			t.Fatalf("Validate(%q) error = %v, want ErrInvalidID", id, err)
		}
	}
}

func TestNewIDUsesValidGeneratedFormat(t *testing.T) {
	id, err := NewID()
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	if err := id.Validate(); err != nil {
		t.Fatalf("generated ID %q validation: %v", id, err)
	}
}

func TestNewCreateRequestGeneratesIDIndependentlyOfName(t *testing.T) {
	request, err := NewCreateRequest("ecommerce-platform")
	if err != nil {
		t.Fatalf("NewCreateRequest: %v", err)
	}
	if request.Name != "ecommerce-platform" {
		t.Fatalf("request name = %q, want %q", request.Name, "ecommerce-platform")
	}
	if err := request.ID.Validate(); err != nil {
		t.Fatalf("request ID %q validation: %v", request.ID, err)
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
	want := filepath.Join(xdgDataHome, "spool")
	if root != want {
		t.Fatalf("StorageRoot = %q, want %q", root, want)
	}
}

func TestStorageRootRejectsRelativeXDGDataHome(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "relative-data")
	if _, err := StorageRoot(); !errors.Is(err, ErrInvalidStorageRoot) {
		t.Fatalf("StorageRoot error = %v, want ErrInvalidStorageRoot", err)
	}
}

func TestStorageRootPlatformDefaults(t *testing.T) {
	tests := []struct {
		name string
		goos string
		env  map[string]string
		want string
	}{
		{
			name: "unix xdg data default",
			goos: "linux",
			want: filepath.Join("/home/alice", ".local", "share", "spool"),
		},
		{
			name: "macos xdg data default",
			goos: "darwin",
			want: filepath.Join("/home/alice", ".local", "share", "spool"),
		},
		{
			name: "windows local app data default",
			goos: "windows",
			want: filepath.Join("/home/alice", "AppData", "Local", "spool"),
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root, err := storageRoot(func(key string) string { return test.env[key] }, "/home/alice", test.goos)
			if err != nil {
				t.Fatalf("storageRoot: %v", err)
			}
			if root != test.want {
				t.Fatalf("storageRoot = %q, want %q", root, test.want)
			}
		})
	}
}

func TestRepositoryPathIsDeterministicAndContainedByStorageRoot(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "workspace-test", "state", "spool")
	id := ID("ws_8f1e2a3b")

	first, err := RepositoryPath(root, id)
	if err != nil {
		t.Fatalf("first RepositoryPath: %v", err)
	}
	second, err := RepositoryPath(root, id)
	if err != nil {
		t.Fatalf("second RepositoryPath: %v", err)
	}
	if first != second {
		t.Fatalf("repository paths differ: %q != %q", first, second)
	}
	relative, err := filepath.Rel(root, first)
	if err != nil {
		t.Fatalf("Rel: %v", err)
	}
	if relative == ".." || len(relative) > 3 && relative[:3] == ".."+string(filepath.Separator) {
		t.Fatalf("state directory %q escapes storage root %q", first, root)
	}
	want := filepath.Join(root, "repos", string(id))
	if first != want {
		t.Fatalf("RepositoryPath = %q, want %q", first, want)
	}
}

func TestResolveCreateLocationUsesXDGDataHome(t *testing.T) {
	xdgDataHome := filepath.Join(string(filepath.Separator), "workspace-test", "isolated-data")
	t.Setenv("XDG_DATA_HOME", xdgDataHome)
	request := CreateRequest{Name: "ecommerce-platform", ID: "ws_8f1e2a3b"}
	location, err := ResolveCreateLocation(request)
	if err != nil {
		t.Fatalf("ResolveCreateLocation: %v", err)
	}
	wantRoot := filepath.Join(xdgDataHome, "spool")
	if location.StorageRoot != wantRoot {
		t.Fatalf("storage root = %q, want %q", location.StorageRoot, wantRoot)
	}
	wantStateDir := filepath.Join(wantRoot, "repos", "ws_8f1e2a3b")
	if location.StateDir != wantStateDir {
		t.Fatalf("state directory = %q, want %q", location.StateDir, wantStateDir)
	}
	if location.ID != request.ID {
		t.Fatalf("workspace ID = %q, want %q", location.ID, request.ID)
	}
}
