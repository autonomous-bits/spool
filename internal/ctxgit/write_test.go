package ctxgit

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository"
)

func TestUnboundSessionRefusesWrites(t *testing.T) {
	t.Parallel()
	var session *Session
	if _, err := session.Stage([]repository.MutationOperation{{
		Action: "add", Entity: "node", ID: "n1", Title: "Nope",
	}}); !errors.Is(err, ErrUnbound) {
		t.Fatalf("Stage error = %v, want ErrUnbound", err)
	}
}

func TestWritePathBranchCommitPRAndProjection(t *testing.T) {
	ctx := context.Background()
	codeRoot, remote, cache := setupBoundWorkspace(t)
	recorder := &RecordingPROpener{}
	session, err := Start(ctx, Options{
		WorkspaceDir: codeRoot,
		CacheDir:     cache,
		Git:          isolatedGit(),
		PROpener:     recorder,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if session.ProjectionInsideCheckout() {
		t.Fatal("projection must not live inside the context checkout")
	}
	status, err := session.ProjectionStatus()
	if err != nil || status.State != "ready" {
		t.Fatalf("projection status = %#v, err=%v", status, err)
	}

	_, err = session.Stage([]repository.MutationOperation{
		{Action: "add", Entity: "node", ID: "idea-1", Title: "Shared idea", Labels: []string{"Requirement"}},
	})
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	result, err := session.Commit(ctx, "agent <agent@example.com>", "Add shared idea")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if !strings.HasPrefix(result.Branch, "spool/mcp/") {
		t.Fatalf("short-lived branch = %q", result.Branch)
	}
	if result.Commit == "" || result.PR.URL == "" {
		t.Fatalf("result = %#v", result)
	}
	if result.ProtectedBranch != "main" {
		t.Fatalf("protected branch = %q", result.ProtectedBranch)
	}
	if len(recorder.Requests) != 1 || recorder.Requests[0].Base != "main" {
		t.Fatalf("PR requests = %#v", recorder.Requests)
	}
	if recorder.Requests[0].Head != result.Branch {
		t.Fatalf("PR head = %q, want %q", recorder.Requests[0].Head, result.Branch)
	}

	node, ok := session.ResolveNode("idea-1")
	if !ok || node.ID != "demo-repo/idea-1" {
		t.Fatalf("namespaced resolve = %#v ok=%v", node, ok)
	}

	checkout := cloneAt(t, remote, result.Branch)
	nodePath := filepath.Join(checkout, "nodes", "demo-repo--idea-1.json")
	data, err := os.ReadFile(nodePath)
	if err != nil {
		t.Fatalf("read node json: %v", err)
	}
	if !json.Valid(data) || !strings.Contains(string(data), "\n  \"id\"") {
		t.Fatalf("node file is not human-diffable JSON: %s", data)
	}
	var decoded repository.Node
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode node: %v", err)
	}
	if decoded.ID != "demo-repo/idea-1" || decoded.Title != "Shared idea" {
		t.Fatalf("decoded node = %#v", decoded)
	}
	if _, err := os.Stat(filepath.Join(checkout, "projection.db")); !os.IsNotExist(err) {
		t.Fatal("projection.db must not be committed to the context remote")
	}

	// Overlap: revising an ID that already exists on the protected checkout
	// still opens a PR so humans review it — no silent overwrite.
	session, err = Start(ctx, Options{WorkspaceDir: codeRoot, CacheDir: cache, Git: isolatedGit(), PROpener: recorder})
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	if _, err := session.Stage([]repository.MutationOperation{
		{Action: "add", Entity: "node", ID: "coderepo", Title: "demo-repo (revised)", Labels: []string{"CodeRepository"}},
	}); err != nil {
		t.Fatalf("overlap stage: %v", err)
	}
	overlap, err := session.Commit(ctx, "agent", "Revise shared idea")
	if err != nil {
		t.Fatalf("overlap commit: %v", err)
	}
	if len(overlap.Overlaps) == 0 {
		t.Fatal("expected overlap to be reported for human PR review")
	}
	if !strings.Contains(recorder.Requests[1].Body, "Overlaps") {
		t.Fatalf("PR body missing overlap section: %s", recorder.Requests[1].Body)
	}

	protected := cloneAt(t, remote, "main")
	if _, err := os.Stat(filepath.Join(protected, "nodes", "demo-repo--idea-1.json")); !os.IsNotExist(err) {
		t.Fatal("successful write must not push-clean to the protected branch")
	}
}

func TestAssetWriteAndRead(t *testing.T) {
	ctx := context.Background()
	codeRoot, _, cache := setupBoundWorkspace(t)
	recorder := &RecordingPROpener{}
	session, err := Start(ctx, Options{WorkspaceDir: codeRoot, CacheDir: cache, Git: isolatedGit(), PROpener: recorder})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	assetPath := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(assetPath, []byte("reference notes"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}
	added, err := session.StageAsset(assetPath, "notes", "Notes")
	if err != nil {
		t.Fatalf("StageAsset: %v", err)
	}
	if added.Hash == "" || !added.Staged {
		t.Fatalf("asset result = %#v", added)
	}
	if _, err := session.Commit(ctx, "agent", "Add notes asset"); err != nil {
		t.Fatalf("Commit asset: %v", err)
	}
	reader, size, _, err := session.ReadAsset(ctx, added.Node)
	if err != nil {
		t.Fatalf("ReadAsset: %v", err)
	}
	got, err := io.ReadAll(reader)
	_ = reader.Close()
	if err != nil {
		t.Fatalf("read asset body: %v", err)
	}
	if size != int64(len(got)) || string(got) != "reference notes" {
		t.Fatalf("asset body = %q size=%d", got, size)
	}
	reader2, _, _, err := session.ReadAsset(ctx, added.Hash)
	if err != nil {
		t.Fatalf("ReadAsset by hash: %v", err)
	}
	_ = reader2.Close()
}

func TestSchemaMigrateNoOpDoesNotOpenPR(t *testing.T) {
	ctx := context.Background()
	codeRoot, _, cache := setupBoundWorkspace(t)
	recorder := &RecordingPROpener{}
	session, err := Start(ctx, Options{WorkspaceDir: codeRoot, CacheDir: cache, Git: isolatedGit(), PROpener: recorder})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	result, err := session.MigrateSchema(ctx, SchemaMigrateRequest{
		SchemaTOML: []byte(defaultSchemaTOML),
		Message:    "Identical schema",
	})
	if err != nil {
		t.Fatalf("MigrateSchema no-op: %v", err)
	}
	if result.PR.URL != "" || len(recorder.Requests) != 0 {
		t.Fatalf("identical schema must not open a PR: %#v requests=%#v", result, recorder.Requests)
	}
	if result.Branch != "main" {
		t.Fatalf("no-op branch = %q, want protected main", result.Branch)
	}
	foundNoChanges := false
	for _, warning := range result.Warnings {
		if strings.Contains(warning, "no changes") {
			foundNoChanges = true
			break
		}
	}
	if !foundNoChanges {
		t.Fatalf("no-op warnings = %#v, want no changes", result.Warnings)
	}
}

func setupBoundWorkspace(t *testing.T) (codeRoot, remote, cache string) {
	t.Helper()
	root := t.TempDir()
	remote = filepath.Join(root, "remote.git")
	if _, err := isolatedGit().Run(context.Background(), root, "init", "--bare", "-b", "main", remote); err != nil {
		t.Fatalf("bare remote: %v", err)
	}
	codeRoot = filepath.Join(root, "code")
	if err := os.MkdirAll(codeRoot, 0o755); err != nil {
		t.Fatalf("mkdir code: %v", err)
	}
	cache = filepath.Join(root, "cache")
	t.Setenv("SPOOL_CONTEXT_CACHE", cache)
	if _, err := Init(context.Background(), InitRequest{
		WorkspaceDir:    codeRoot,
		Remote:          remote,
		SolutionID:      "demo",
		ProtectedBranch: "main",
		RepositoryID:    "demo-repo",
		Author:          "tester <tester@example.com>",
	}, isolatedGit()); err != nil {
		t.Fatalf("Init: %v", err)
	}
	return codeRoot, remote, filepath.Join(cache, "demo")
}

func cloneAt(t *testing.T, remote, branch string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "clone")
	if _, err := isolatedGit().Run(context.Background(), t.TempDir(), "clone", "--branch", branch, remote, dir); err != nil {
		t.Fatalf("clone %s: %v", branch, err)
	}
	return dir
}

func isolatedGit() GitRunner {
	return GitRunner{
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
