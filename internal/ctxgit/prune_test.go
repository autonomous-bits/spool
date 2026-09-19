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

func TestPruneBoundRemovesEphemeralAndOpensPR(t *testing.T) {
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
	if _, err := session.Stage([]repository.MutationOperation{
		{Action: "add", Entity: "node", ID: "durable-1", Title: "Durable 1", Labels: []string{"Architecture", "Component"}},
		{Action: "add", Entity: "node", ID: "ephemeral-1", Title: "Ephemeral 1", Labels: []string{"Architecture", "Ephemeral"}},
		{Action: "add", Entity: "edge", ID: "edge-1", Source: "durable-1", Target: "ephemeral-1", Type: "DEPENDS_ON"},
	}); err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if _, err := session.Commit(ctx, "tester", "Add prune fixtures"); err != nil {
		t.Fatalf("Commit fixtures: %v", err)
	}

	// Simulate merge of the fixture PR onto protected so prune sees the nodes.
	pushMerged(t, remote, recorder.Requests[len(recorder.Requests)-1].Head)

	session, err = Start(ctx, Options{WorkspaceDir: codeRoot, CacheDir: cache, Git: isolatedGit(), PROpener: recorder})
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	preview, err := session.Prune(ctx, PruneRequest{DryRun: true})
	if err != nil {
		t.Fatalf("dry-run prune: %v", err)
	}
	if !preview.DryRun || preview.PrunedNodesCount != 1 || preview.PrunedEdgesCount != 1 {
		t.Fatalf("dry-run = %#v", preview)
	}
	if preview.PullRequest.URL != "" {
		t.Fatal("dry-run must not open a PR")
	}

	result, err := session.Prune(ctx, PruneRequest{Author: "alice", Message: "Prune ephemeral"})
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if result.DryRun || result.PrunedNodesCount != 1 || result.PrunedEdgesCount != 1 {
		t.Fatalf("prune = %#v", result)
	}
	if result.Commit == "" || result.PullRequest.URL == "" || !strings.HasPrefix(result.Branch, "spool/mcp/") {
		t.Fatalf("prune write path = %#v", result)
	}
	checkout := cloneAt(t, remote, result.Branch)
	if _, err := os.Stat(filepath.Join(checkout, "nodes", "demo-repo--ephemeral-1.json")); !os.IsNotExist(err) {
		t.Fatal("ephemeral node file should be deleted on the prune branch")
	}
	if _, err := os.Stat(filepath.Join(checkout, "nodes", "demo-repo--durable-1.json")); err != nil {
		t.Fatalf("durable node missing: %v", err)
	}
}

func TestPruneUnboundRefused(t *testing.T) {
	var session *Session
	_, err := session.Prune(context.Background(), PruneRequest{})
	if !errors.Is(err, ErrUnbound) {
		t.Fatalf("error = %v, want ErrUnbound", err)
	}
}

func TestPruneFailClosedWhenBindRemoved(t *testing.T) {
	ctx := context.Background()
	codeRoot, _, cache := setupBoundWorkspace(t)
	session, err := Start(ctx, Options{WorkspaceDir: codeRoot, CacheDir: cache, Git: isolatedGit(), PROpener: &RecordingPROpener{}})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := os.Remove(session.BindPath); err != nil {
		t.Fatalf("remove bind: %v", err)
	}
	_, err = session.Prune(ctx, PruneRequest{DryRun: true})
	if !errors.Is(err, ErrUnbound) {
		t.Fatalf("error = %v, want ErrUnbound after bind removal", err)
	}
}

func TestPruneRebuildsProjectionAndLeavesSplUntouched(t *testing.T) {
	ctx := context.Background()
	codeRoot, remote, cache := setupBoundWorkspace(t)
	leftover := filepath.Join(codeRoot, ".spl", "objects", "pack")
	if err := os.MkdirAll(leftover, 0o755); err != nil {
		t.Fatalf("mkdir leftover: %v", err)
	}
	marker := filepath.Join(leftover, "do-not-gc")
	if err := os.WriteFile(marker, []byte("cas-pack-bytes"), 0o644); err != nil {
		t.Fatalf("write leftover pack: %v", err)
	}

	recorder := &RecordingPROpener{}
	session, err := Start(ctx, Options{WorkspaceDir: codeRoot, CacheDir: cache, Git: isolatedGit(), PROpener: recorder})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := session.Stage([]repository.MutationOperation{
		{Action: "add", Entity: "node", ID: "ephemeral-keep-cut", Title: "Temp", Labels: []string{"Ephemeral"}},
	}); err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if _, err := session.Commit(ctx, "tester", "Add ephemeral"); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	pushMerged(t, remote, recorder.Requests[len(recorder.Requests)-1].Head)

	session, err = Start(ctx, Options{WorkspaceDir: codeRoot, CacheDir: cache, Git: isolatedGit(), PROpener: recorder})
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	if session.projectionPath == "" {
		t.Fatal("expected projection after Start")
	}
	if err := os.Remove(session.projectionPath); err != nil {
		t.Fatalf("remove projection: %v", err)
	}

	result, err := session.Prune(ctx, PruneRequest{Author: "alice", Message: "Prune ephemeral"})
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if result.PrunedNodesCount != 1 || result.PullRequest.URL == "" {
		t.Fatalf("prune = %#v", result)
	}
	status, err := session.ProjectionStatus()
	if err != nil {
		t.Fatalf("ProjectionStatus: %v", err)
	}
	if status.State != "ready" || status.Path == "" {
		t.Fatalf("projection after prune = %#v", status)
	}
	if _, err := os.Stat(status.Path); err != nil {
		t.Fatalf("rebuilt projection missing: %v", err)
	}
	got, err := os.ReadFile(marker)
	if err != nil || string(got) != "cas-pack-bytes" {
		t.Fatalf("leftover .spl pack mutated: err=%v content=%q", err, got)
	}
}

func TestPruneSourceNeverInvokesCASGC(t *testing.T) {
	src, err := os.ReadFile("prune.go")
	if err != nil {
		t.Fatalf("read prune.go: %v", err)
	}
	text := string(src)
	for _, needle := range []string{
		"internal/repository/prune",
		"internal/repository/gc",
		`"gc"`,
		`"repack"`,
		`"pack-objects"`,
		"git gc",
	} {
		if strings.Contains(text, needle) {
			t.Errorf("graph prune must not invoke CAS/pack GC; found %q", needle)
		}
	}
}

func TestQueryContextBoundSearchExpand(t *testing.T) {
	ctx := context.Background()
	codeRoot, remote, cache := setupBoundWorkspace(t)
	recorder := &RecordingPROpener{}
	session, err := Start(ctx, Options{WorkspaceDir: codeRoot, CacheDir: cache, Git: isolatedGit(), PROpener: recorder})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := session.Stage([]repository.MutationOperation{
		{Action: "add", Entity: "node", ID: "seed-a", Title: "evidence alpha", Labels: []string{"Seed"}},
		{Action: "add", Entity: "node", ID: "related", Title: "related"},
		{Action: "add", Entity: "edge", ID: "related-edge", Source: "seed-a", Target: "related", Type: "RELATED"},
	}); err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if _, err := session.Commit(ctx, "tester", "Add query fixtures"); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	pushMerged(t, remote, recorder.Requests[0].Head)
	session, err = Start(ctx, Options{WorkspaceDir: codeRoot, CacheDir: cache, Git: isolatedGit(), PROpener: recorder})
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	resolved, err := session.ResolveResult("seed-a")
	if err != nil || resolved.Node.Title != "evidence alpha" {
		t.Fatalf("resolve = %#v err=%v", resolved, err)
	}
	filtered, err := session.FilterResult([]string{"Seed"}, nil, 10)
	if err != nil || len(filtered.Nodes) != 1 {
		t.Fatalf("filter = %#v err=%v", filtered, err)
	}
	expanded, err := session.QueryContext(ctx, QueryContextRequest{
		Labels: []string{"Seed"}, Direction: "both", EdgeTypes: []string{"RELATED"},
	})
	if err != nil {
		t.Fatalf("query-context: %v", err)
	}
	if len(expanded.Evidence) != 1 || len(expanded.Nodes) < 2 {
		t.Fatalf("expanded = %#v", expanded)
	}
	data, _ := json.Marshal(expanded)
	if !json.Valid(data) {
		t.Fatal("query-context JSON invalid")
	}
}

func TestMergePreviewApplyCleanFileGraph(t *testing.T) {
	ctx := context.Background()
	codeRoot, remote, cache := setupBoundWorkspace(t)
	recorder := &RecordingPROpener{}
	session, err := Start(ctx, Options{WorkspaceDir: codeRoot, CacheDir: cache, Git: isolatedGit(), PROpener: recorder})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if _, err := session.Stage([]repository.MutationOperation{
		{Action: "add", Entity: "node", ID: "feature-node", Title: "Feature Node", Labels: []string{"Requirement"}},
	}); err != nil {
		t.Fatalf("Stage: %v", err)
	}
	write, err := session.Commit(ctx, "tester", "Add feature node")
	if err != nil {
		t.Fatalf("Commit: %v", err)
	}

	// Keep the feature commit as a named git branch on the remote; protected stays unchanged.
	if _, err := isolatedGit().Run(ctx, session.CheckoutDir, "push", "origin", write.Branch+":refs/heads/feature"); err != nil {
		t.Fatalf("publish feature: %v", err)
	}

	preview, err := session.PreviewMerge(ctx, "feature", "main")
	if err != nil {
		t.Fatalf("PreviewMerge: %v", err)
	}
	if !preview.Clean || preview.ID == "" {
		t.Fatalf("preview = %#v", preview)
	}
	apply, _, err := session.ApplyMerge(ctx, "feature", "main", "tx-1", preview.ID, "alice", "Merge feature")
	if err != nil {
		t.Fatalf("ApplyMerge: %v", err)
	}
	if apply.Commit == "" || apply.PR.URL == "" {
		t.Fatalf("apply = %#v", apply)
	}
	checkout := cloneAt(t, remote, apply.Branch)
	if _, err := os.Stat(filepath.Join(checkout, "nodes", "demo-repo--feature-node.json")); err != nil {
		t.Fatalf("merged node missing: %v", err)
	}
}

func pushMerged(t *testing.T, remote, head string) {
	t.Helper()
	dir := cloneAt(t, remote, head)
	git := isolatedGit()
	ctx := context.Background()
	if _, err := git.Run(ctx, dir, "push", "origin", "HEAD:main"); err != nil {
		t.Fatalf("merge to protected: %v", err)
	}
}
