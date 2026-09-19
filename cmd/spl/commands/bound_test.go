package commands

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/autonomous-bits/spool/internal/ctxgit"
)

func boundCommandOptions(t *testing.T) ctxgit.Options {
	t.Helper()
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	git := isolatedCommandGit()
	if _, err := git.Run(context.Background(), root, "init", "--bare", "-b", "main", remote); err != nil {
		t.Fatalf("bare remote: %v", err)
	}
	code := filepath.Join(root, "code")
	if err := os.MkdirAll(code, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	cache := filepath.Join(root, "cache")
	t.Setenv("SPOOL_CONTEXT_CACHE", cache)
	if _, err := ctxgit.Init(context.Background(), ctxgit.InitRequest{
		WorkspaceDir:    code,
		Remote:          remote,
		SolutionID:      "cli-demo",
		ProtectedBranch: "main",
		RepositoryID:    "cli-repo",
		Author:          "tester <tester@example.com>",
	}, git); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return ctxgit.Options{
		WorkspaceDir: code,
		CacheDir:     filepath.Join(cache, "cli-demo"),
		Git:          git,
		PROpener:     &ctxgit.RecordingPROpener{},
	}
}

func isolatedCommandGit() ctxgit.GitRunner {
	return ctxgit.GitRunner{
		Env: []string{
			"GIT_CONFIG_GLOBAL=/dev/null",
			"GIT_CONFIG_SYSTEM=/dev/null",
			"GIT_AUTHOR_NAME=spool-test",
			"GIT_AUTHOR_EMAIL=spool-test@example.com",
			"GIT_COMMITTER_NAME=spool-test",
			"GIT_COMMITTER_EMAIL=spool-test@example.com",
		},
	}
}
