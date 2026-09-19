package ctxgit

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

func TestFindBindIgnoresSplGoWorkAndRackLeftovers(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".spl"), 0o755); err != nil {
		t.Fatalf("mkdir .spl: %v", err)
	}
	rack := []byte("format_version = 2\n\n[remote]\nendpoint = \"https://rack.example.invalid\"\nauth_mode = \"bearer\"\nworkspace_id = \"leftover\"\n")
	if err := os.WriteFile(filepath.Join(root, ".spl", "config.toml"), rack, 0o644); err != nil {
		t.Fatalf("write rack leftover: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.work"), []byte("go 1.26\n"), 0o644); err != nil {
		t.Fatalf("write go.work: %v", err)
	}
	_, _, _, err := FindBind(root)
	if !errors.Is(err, ErrUnbound) {
		t.Fatalf("FindBind error = %v, want ErrUnbound (no bind from .spl/go.work/Rack)", err)
	}
}

func TestFindBindDoesNotInferRemoteFromDirectoryName(t *testing.T) {
	t.Parallel()
	root := filepath.Join(t.TempDir(), "github.com", "org", "fancy-service")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, _, _, err := FindBind(root)
	if !errors.Is(err, ErrUnbound) {
		t.Fatalf("FindBind error = %v, want ErrUnbound (directory name is not a remote)", err)
	}
}

func TestFindBindStopsAtNestedGitWorkTree(t *testing.T) {
	t.Parallel()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required")
	}
	root := t.TempDir()
	parent := filepath.Join(root, "monorepo")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatalf("mkdir parent: %v", err)
	}
	if _, err := WriteBindFile(parent, Bind{
		SolutionID:      "mono",
		Remote:          "https://github.com/org/mono-context.git",
		ProtectedBranch: "main",
		RepositoryID:    "monorepo",
	}); err != nil {
		t.Fatalf("WriteBindFile: %v", err)
	}
	nested := filepath.Join(parent, "service-a")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir nested: %v", err)
	}
	git := isolatedGit()
	if _, err := git.Run(context.Background(), nested, "init", "-b", "main"); err != nil {
		t.Fatalf("git init nested: %v", err)
	}
	if _, err := git.Run(context.Background(), nested, "remote", "add", "origin", "https://github.com/org/service-a.git"); err != nil {
		t.Fatalf("git remote: %v", err)
	}
	_, _, _, err := FindBind(nested)
	if !errors.Is(err, ErrUnbound) {
		t.Fatalf("FindBind error = %v, want ErrUnbound (nested git repo must not inherit parent bind or origin URL)", err)
	}
}

func TestRefuseLegacySoTWhenBound(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	if err := RefuseLegacySoT(root, "spl remote set"); err != nil {
		t.Fatalf("unbound RefuseLegacySoT = %v", err)
	}
	if _, err := WriteBindFile(root, Bind{
		SolutionID: "demo",
		Remote:     "https://github.com/org/demo-context.git",
	}); err != nil {
		t.Fatalf("WriteBindFile: %v", err)
	}
	err := RefuseLegacySoT(root, "spl remote set")
	if err == nil {
		t.Fatal("bound RefuseLegacySoT = nil, want error")
	}
	if !strings.Contains(err.Error(), BindRelPath) || !strings.Contains(err.Error(), "spl context init --remote") {
		t.Fatalf("error = %v, want bind+context git guidance", err)
	}
}
