package ctxgit

import "testing"

func TestForbiddenContextPath(t *testing.T) {
	t.Parallel()
	for _, path := range []string{
		"objects/pack/0001.pack",
		".spl/config.toml",
		"logs/HEAD",
		"merge/lease.json",
		"graph.db",
		"projection.db",
		"nodes/graph.db",
		"assets/foo.pack",
	} {
		if !forbiddenContextPath(path) {
			t.Fatalf("%s should be forbidden", path)
		}
	}
	for _, path := range []string{
		"schema.toml",
		"nodes/demo-repo--idea-1.json",
		"edges/demo-repo--rel-1.json",
		"assets/abc/notes.txt",
		"README.md",
		".gitignore",
	} {
		if forbiddenContextPath(path) {
			t.Fatalf("%s should be allowed", path)
		}
	}
}

func TestRejectForbiddenPaths(t *testing.T) {
	t.Parallel()
	if err := rejectForbiddenPaths([]string{"nodes/a.json", "schema.toml"}); err != nil {
		t.Fatalf("allowed paths: %v", err)
	}
	if err := rejectForbiddenPaths([]string{"nodes/a.json", "objects/pack/x.pack"}); err == nil {
		t.Fatal("expected forbidden pack path")
	}
}
