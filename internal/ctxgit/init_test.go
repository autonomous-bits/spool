package ctxgit

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository"
)

func TestInitSeedsCodeRepositoryFromBindNotSiblings(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	if _, err := isolatedGit().Run(ctx, root, "init", "--bare", "-b", "main", remote); err != nil {
		t.Fatalf("bare remote: %v", err)
	}
	cache := filepath.Join(root, "cache")
	t.Setenv("SPOOL_CONTEXT_CACHE", cache)

	repoA := filepath.Join(root, "svc-a")
	repoB := filepath.Join(root, "svc-b")
	if err := os.MkdirAll(repoA, 0o755); err != nil {
		t.Fatalf("mkdir a: %v", err)
	}
	if err := os.MkdirAll(repoB, 0o755); err != nil {
		t.Fatalf("mkdir b: %v", err)
	}
	// A sibling bind must not be auto-discovered when initializing repo A.
	if _, err := WriteBindFile(repoB, Bind{
		SolutionID:      "demo",
		Remote:          remote,
		ProtectedBranch: "main",
		RepositoryID:    "svc-b",
	}); err != nil {
		t.Fatalf("sibling bind: %v", err)
	}

	first, err := Init(ctx, InitRequest{
		WorkspaceDir:    repoA,
		Remote:          remote,
		SolutionID:      "demo",
		ProtectedBranch: "main",
		RepositoryID:    "svc-a",
		Author:          "tester <tester@example.com>",
	}, isolatedGit())
	if err != nil {
		t.Fatalf("init a: %v", err)
	}
	if first.SeededNode != "svc-a/coderepo" {
		t.Fatalf("first seeded = %q", first.SeededNode)
	}

	checkout := cloneAt(t, remote, "main")
	if _, err := os.Stat(filepath.Join(checkout, "schema.toml")); err != nil {
		t.Fatalf("schema.toml: %v", err)
	}
	if _, err := os.Stat(filepath.Join(checkout, "nodes")); err != nil {
		t.Fatalf("nodes/: %v", err)
	}
	assertCodeRepository(t, checkout, "svc-a/coderepo", "svc-a")
	if _, err := os.Stat(filepath.Join(checkout, "nodes", "svc-b--coderepo.json")); !os.IsNotExist(err) {
		t.Fatal("sibling bind must not seed a CodeRepository during the other repo's init")
	}

	second, err := Init(ctx, InitRequest{
		WorkspaceDir:    repoB,
		Remote:          remote,
		SolutionID:      "demo",
		ProtectedBranch: "main",
		RepositoryID:    "svc-b",
		Author:          "tester <tester@example.com>",
	}, isolatedGit())
	if err != nil {
		t.Fatalf("init b: %v", err)
	}
	if second.SeededNode != "svc-b/coderepo" {
		t.Fatalf("second seeded = %q", second.SeededNode)
	}

	checkout = cloneAt(t, remote, "main")
	assertCodeRepository(t, checkout, "svc-a/coderepo", "svc-a")
	assertCodeRepository(t, checkout, "svc-b/coderepo", "svc-b")
}

func assertCodeRepository(t *testing.T, checkout, id, repositoryID string) {
	t.Helper()
	name, err := FileName(id)
	if err != nil {
		t.Fatalf("FileName: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(checkout, "nodes", name))
	if err != nil {
		t.Fatalf("read %s: %v", id, err)
	}
	var node repository.Node
	if err := json.Unmarshal(data, &node); err != nil {
		t.Fatalf("decode %s: %v", id, err)
	}
	if node.ID != id || !containsLabel(node.Labels, "CodeRepository") {
		t.Fatalf("node %s = %#v", id, node)
	}
	if got := node.Properties["repositoryId"].String; got != repositoryID {
		t.Fatalf("%s repositoryId = %q", id, got)
	}
	if strings.TrimSpace(node.Properties["remote"].String) == "" {
		t.Fatalf("%s missing remote property", id)
	}
}

func containsLabel(labels []string, want string) bool {
	for _, label := range labels {
		if label == want {
			return true
		}
	}
	return false
}
