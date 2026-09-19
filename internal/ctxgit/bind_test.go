package ctxgit

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFindBindUnbound(t *testing.T) {
	t.Parallel()
	_, _, _, err := FindBind(t.TempDir())
	if !errors.Is(err, ErrUnbound) {
		t.Fatalf("FindBind error = %v, want ErrUnbound", err)
	}
}

func TestLoadAndFindBind(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if _, err := WriteBindFile(root, Bind{
		SolutionID:      "demo",
		Remote:          "https://github.com/org/demo-context.git",
		ProtectedBranch: "main",
		RepositoryID:    "code-a",
	}); err != nil {
		t.Fatalf("WriteBindFile: %v", err)
	}
	nested := filepath.Join(root, "pkg", "api")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	codeRoot, path, bind, err := FindBind(nested)
	if err != nil {
		t.Fatalf("FindBind: %v", err)
	}
	if codeRoot != root {
		t.Fatalf("code root = %q, want %q", codeRoot, root)
	}
	if filepath.Base(filepath.Dir(path)) != ".spool" {
		t.Fatalf("bind path = %q", path)
	}
	if bind.SolutionID != "demo" || bind.Remote == "" || bind.RepositoryID != "code-a" {
		t.Fatalf("bind = %#v", bind)
	}
	if bind.LFSThreshold() != DefaultLFSThreshold {
		t.Fatalf("LFS threshold = %d", bind.LFSThreshold())
	}
}

func TestLoadBindMissingRemote(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "context.toml")
	if err := os.WriteFile(path, []byte("solution_id = \"x\"\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := LoadBindFile(path); !errors.Is(err, ErrInvalidBind) {
		t.Fatalf("LoadBindFile error = %v, want ErrInvalidBind", err)
	}
}
