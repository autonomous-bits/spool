package ctxgit

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository"
)

func TestExportRefusesUnbound(t *testing.T) {
	t.Parallel()
	var session *Session
	_, err := session.Export(context.Background(), ExportRequest{Repo: &repository.Repository{}})
	if !errors.Is(err, ErrUnbound) {
		t.Fatalf("Export error = %v, want ErrUnbound", err)
	}
}

func TestExportKeepDropPRPathAndNoRackSideEffect(t *testing.T) {
	ctx := context.Background()
	codeRoot, remote, cache := setupBoundWorkspace(t)
	stateDir := filepath.Join(codeRoot, ".spl")
	repo := newSplGraph(t, stateDir)
	plantDroppedArtifacts(t, stateDir)
	if err := repo.SetRemote(repository.RemoteConfig{
		Endpoint: "http://127.0.0.1:1",
		RepoID:   "acme",
		AuthMode: repository.RemoteAuthModeBearer,
	}); err != nil {
		t.Fatalf("SetRemote: %v", err)
	}

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

	result, err := session.Export(ctx, ExportRequest{
		Repo:    repo,
		Branch:  "main",
		Author:  "migrator <migrator@example.com>",
		Message: "One-shot export",
	})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if result.Entrypoint != exportEntrypoint || !result.Lossy || !result.OverwriteAtOwnRisk {
		t.Fatalf("honesty fields = %#v", result)
	}
	if !strings.HasPrefix(result.Branch, "spool/mcp/") || result.PR.URL == "" {
		t.Fatalf("PR path = %#v", result.WriteResult)
	}
	if strings.Contains(strings.ToLower(result.Note), "sync from rack") || strings.Contains(result.Entrypoint, "sync") {
		t.Fatalf("entrypoint/note must not say sync: %#v", result)
	}
	assertContainsAll(t, result.Kept, "nodes (", "edges (", "schema", "assets (")
	assertContainsAll(t, result.Skipped, "packs", "Rack remotes", "reflogs", "merge leases", "projections")
	if !result.RackRemoteUnchanged {
		t.Fatal("Rack remote must be unchanged")
	}
	cfg, ok, err := repo.Remote()
	if err != nil || !ok || cfg.Endpoint != "http://127.0.0.1:1" {
		t.Fatalf("Rack remote mutated: ok=%v cfg=%#v err=%v", ok, cfg, err)
	}
	if len(recorder.Requests) != 1 || recorder.Requests[0].Base != "main" {
		t.Fatalf("PR requests = %#v", recorder.Requests)
	}
	body := recorder.Requests[0].Body
	if !strings.Contains(body, "migrate-once") || strings.Contains(strings.ToLower(body), "sync from") {
		t.Fatalf("PR body = %s", body)
	}
	if !strings.Contains(body, "## Kept") || !strings.Contains(body, "## Skipped") {
		t.Fatalf("PR body missing kept/skipped: %s", body)
	}

	checkout := cloneAt(t, remote, result.Branch)
	nodePath := filepath.Join(checkout, "nodes", "demo-repo--idea-1.json")
	data, err := os.ReadFile(nodePath)
	if err != nil {
		t.Fatalf("exported node: %v", err)
	}
	var decoded repository.Node
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode node: %v", err)
	}
	if decoded.ID != "demo-repo/idea-1" || decoded.Title != "Shared idea" {
		t.Fatalf("decoded node = %#v", decoded)
	}
	if _, err := os.Stat(filepath.Join(checkout, "schema.toml")); err != nil {
		t.Fatalf("schema.toml: %v", err)
	}
	if _, err := os.Stat(filepath.Join(checkout, "edges", "demo-repo--rel-1.json")); err != nil {
		t.Fatalf("edge: %v", err)
	}
	assertNoForbiddenTree(t, checkout)

	protected := cloneAt(t, remote, "main")
	if _, err := os.Stat(filepath.Join(protected, "nodes", "demo-repo--idea-1.json")); !os.IsNotExist(err) {
		t.Fatal("export must not push-clean to the protected branch")
	}
}

func newSplGraph(t *testing.T, stateDir string) *repository.Repository {
	t.Helper()
	repo, err := repository.InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if _, err := repo.StageMutationBatch(repository.StageMutationRequest{
		Branch: "main",
		Operations: []repository.MutationOperation{
			{Action: "add", Entity: "node", ID: "idea-1", Title: "Shared idea", Labels: []string{"Requirement"}},
			{Action: "add", Entity: "edge", ID: "rel-1", Source: repository.SeedNodeID, Target: "idea-1", Type: "RELATES_TO"},
		},
	}); err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	if _, err := repo.CommitStagedMutations("main"); err != nil {
		t.Fatalf("commit graph: %v", err)
	}
	assetPath := filepath.Join(t.TempDir(), "notes.txt")
	if err := os.WriteFile(assetPath, []byte("reference notes"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}
	if _, err := repo.StageAsset(context.Background(), repository.AssetAddRequest{
		Branch:   "main",
		FilePath: assetPath,
		Title:    "Notes",
		ID:       "notes",
	}); err != nil {
		t.Fatalf("StageAsset: %v", err)
	}
	if _, err := repo.CommitStagedMutations("main"); err != nil {
		t.Fatalf("commit asset: %v", err)
	}
	return repo
}

func plantDroppedArtifacts(t *testing.T, stateDir string) {
	t.Helper()
	packDir := filepath.Join(stateDir, "objects", "pack")
	if err := os.MkdirAll(packDir, 0o755); err != nil {
		t.Fatalf("mkdir pack: %v", err)
	}
	if err := os.WriteFile(filepath.Join(packDir, "0001.pack"), []byte("pack-bytes"), 0o644); err != nil {
		t.Fatalf("write pack: %v", err)
	}
	logDir := filepath.Join(stateDir, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		t.Fatalf("mkdir logs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(logDir, "HEAD"), []byte("reflog"), 0o644); err != nil {
		t.Fatalf("write reflog: %v", err)
	}
	mergeDir := filepath.Join(stateDir, "merge")
	if err := os.MkdirAll(mergeDir, 0o755); err != nil {
		t.Fatalf("mkdir merge: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mergeDir, "lease.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write lease: %v", err)
	}
}

func assertContainsAll(t *testing.T, items []string, needles ...string) {
	t.Helper()
	joined := strings.Join(items, "\n")
	for _, needle := range needles {
		if !strings.Contains(joined, needle) {
			t.Fatalf("missing %q in %v", needle, items)
		}
	}
}

func assertNoForbiddenTree(t *testing.T, root string) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if rel == "." {
			return nil
		}
		if rel == ".git" || strings.HasPrefix(filepath.ToSlash(rel), ".git/") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if forbiddenContextPath(rel) {
			return errors.New(rel)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("forbidden path in context tree: %v", err)
	}
}
