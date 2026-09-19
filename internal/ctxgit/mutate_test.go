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

func TestMutateUnboundRefused(t *testing.T) {
	t.Parallel()
	var session *Session
	_, err := session.Mutate(context.Background(), MutateRequest{
		Operations: []repository.MutationOperation{{
			Action: "add", Entity: "node", ID: "n1", Title: "Nope",
		}},
	})
	if !errors.Is(err, ErrUnbound) {
		t.Fatalf("error = %v, want ErrUnbound", err)
	}
}

func TestMutateFailClosedWhenBindRemoved(t *testing.T) {
	ctx := context.Background()
	codeRoot, _, cache := setupBoundWorkspace(t)
	session, err := Start(ctx, Options{WorkspaceDir: codeRoot, CacheDir: cache, Git: isolatedGit(), PROpener: &RecordingPROpener{}})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := os.Remove(session.BindPath); err != nil {
		t.Fatalf("remove bind: %v", err)
	}
	_, err = session.Mutate(ctx, MutateRequest{
		Operations: []repository.MutationOperation{{
			Action: "add", Entity: "node", ID: "idea-1", Title: "Shared idea", Labels: []string{"Requirement"},
		}},
		Message: "must fail closed",
	})
	if !errors.Is(err, ErrUnbound) {
		t.Fatalf("error = %v, want ErrUnbound after bind removal", err)
	}
}

func TestMutateEmptyBatchRejected(t *testing.T) {
	ctx := context.Background()
	codeRoot, _, cache := setupBoundWorkspace(t)
	recorder := &RecordingPROpener{}
	session, err := Start(ctx, Options{WorkspaceDir: codeRoot, CacheDir: cache, Git: isolatedGit(), PROpener: recorder})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	_, err = session.Mutate(ctx, MutateRequest{Message: "empty"})
	if !errors.Is(err, repository.ErrInvalidMutationBatch) {
		t.Fatalf("error = %v, want ErrInvalidMutationBatch", err)
	}
	if len(recorder.Requests) != 0 {
		t.Fatalf("empty batch must not open a PR: %#v", recorder.Requests)
	}
}

func TestMutateWritesShortLivedBranchAndPR(t *testing.T) {
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

	result, err := session.Mutate(ctx, MutateRequest{
		Operations: []repository.MutationOperation{
			{Action: "add", Entity: "node", ID: "idea-1", Title: "Shared idea", Labels: []string{"Requirement"}},
			{Action: "add", Entity: "node", ID: "idea-2", Title: "Related idea", Labels: []string{"Requirement"}},
			{Action: "add", Entity: "edge", ID: "idea-2-relates", Source: "idea-2", Target: "idea-1", Type: "RELATES_TO"},
		},
		Author:  "agent <agent@example.com>",
		Message: "Record shared ideas",
	})
	if err != nil {
		t.Fatalf("Mutate: %v", err)
	}
	if result.Operations != 3 {
		t.Fatalf("operations = %d, want 3", result.Operations)
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
	if len(result.Written) == 0 {
		t.Fatalf("WriteResult.Written empty: %#v", result)
	}
	if len(recorder.Requests) != 1 || recorder.Requests[0].Base != "main" || recorder.Requests[0].Head != result.Branch {
		t.Fatalf("PR requests = %#v", recorder.Requests)
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
	if !json.Valid(data) {
		t.Fatalf("node file is not JSON: %s", data)
	}
	var decoded repository.Node
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode node: %v", err)
	}
	if decoded.ID != "demo-repo/idea-1" || decoded.Title != "Shared idea" {
		t.Fatalf("decoded node = %#v", decoded)
	}
	if _, err := os.Stat(filepath.Join(checkout, "edges", "demo-repo--idea-2-relates.json")); err != nil {
		t.Fatalf("edge file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(checkout, "projection.db")); !os.IsNotExist(err) {
		t.Fatal("projection.db must not be committed to the context remote")
	}

	protected := cloneAt(t, remote, "main")
	if _, err := os.Stat(filepath.Join(protected, "nodes", "demo-repo--idea-1.json")); !os.IsNotExist(err) {
		t.Fatal("successful mutate must not push-clean to the protected branch")
	}
}

func TestMutateIdenticalDiffDoesNotOpenPR(t *testing.T) {
	ctx := context.Background()
	codeRoot, remote, cache := setupBoundWorkspace(t)
	recorder := &RecordingPROpener{}
	session, err := Start(ctx, Options{WorkspaceDir: codeRoot, CacheDir: cache, Git: isolatedGit(), PROpener: recorder})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	ops := []repository.MutationOperation{
		{Action: "add", Entity: "node", ID: "idea-1", Title: "Shared idea", Labels: []string{"Requirement"}},
	}
	first, err := session.Mutate(ctx, MutateRequest{Operations: ops, Message: "Record idea"})
	if err != nil {
		t.Fatalf("first Mutate: %v", err)
	}
	if first.PR.URL == "" {
		t.Fatalf("first mutate must open a PR: %#v", first)
	}
	pushMerged(t, remote, first.Branch)

	session, err = Start(ctx, Options{WorkspaceDir: codeRoot, CacheDir: cache, Git: isolatedGit(), PROpener: recorder})
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	second, err := session.Mutate(ctx, MutateRequest{Operations: ops, Message: "Record idea again"})
	if err != nil {
		t.Fatalf("identical Mutate: %v", err)
	}
	if second.PR.URL != "" || len(recorder.Requests) != 1 {
		t.Fatalf("identical mutate must not open a PR: %#v requests=%#v", second, recorder.Requests)
	}
	if second.Branch != "main" {
		t.Fatalf("no-op branch = %q, want protected main", second.Branch)
	}
	foundNoChanges := false
	for _, warning := range second.Warnings {
		if strings.Contains(warning, "no changes") {
			foundNoChanges = true
			break
		}
	}
	if !foundNoChanges {
		t.Fatalf("no-op warnings = %#v, want no changes", second.Warnings)
	}
}

func TestMutateDeleteReportsDeletedAndRebuildsProjection(t *testing.T) {
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
	added, err := session.Mutate(ctx, MutateRequest{
		Operations: []repository.MutationOperation{
			{Action: "add", Entity: "node", ID: "temp-1", Title: "Temporary", Labels: []string{"Requirement"}},
		},
		Message: "Add temporary",
	})
	if err != nil {
		t.Fatalf("add Mutate: %v", err)
	}
	pushMerged(t, remote, added.Branch)

	session, err = Start(ctx, Options{WorkspaceDir: codeRoot, CacheDir: cache, Git: isolatedGit(), PROpener: recorder})
	if err != nil {
		t.Fatalf("restart: %v", err)
	}
	if err := os.Remove(session.projectionPath); err != nil {
		t.Fatalf("remove projection: %v", err)
	}
	deleted, err := session.Mutate(ctx, MutateRequest{
		Operations: []repository.MutationOperation{
			{Action: "delete", Entity: "node", ID: "temp-1"},
		},
		Message: "Remove temporary",
	})
	if err != nil {
		t.Fatalf("delete Mutate: %v", err)
	}
	if deleted.PR.URL == "" || !strings.HasPrefix(deleted.Branch, "spool/mcp/") {
		t.Fatalf("delete mutate write path = %#v", deleted)
	}
	if len(deleted.Deleted) == 0 {
		t.Fatalf("delete mutate missing Deleted paths: %#v", deleted)
	}
	found := false
	for _, path := range deleted.Deleted {
		if strings.Contains(path, "temp-1") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Deleted = %#v, want temp-1 node path", deleted.Deleted)
	}

	status, err := session.ProjectionStatus()
	if err != nil {
		t.Fatalf("ProjectionStatus: %v", err)
	}
	if status.State != "ready" || status.Path == "" {
		t.Fatalf("projection after mutate = %#v", status)
	}
	if _, err := os.Stat(status.Path); err != nil {
		t.Fatalf("rebuilt projection missing: %v", err)
	}
	if _, ok := session.ResolveNode("temp-1"); ok {
		t.Fatal("deleted node still resolvable after mutate")
	}
	got, err := os.ReadFile(marker)
	if err != nil || string(got) != "cas-pack-bytes" {
		t.Fatalf("leftover .spl pack mutated: err=%v content=%q", err, got)
	}
}

func TestMutateSourceNeverInvokesCASGC(t *testing.T) {
	src, err := os.ReadFile("mutate.go")
	if err != nil {
		t.Fatalf("read mutate.go: %v", err)
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
			t.Errorf("mutate must not invoke CAS/pack GC; found %q", needle)
		}
	}
}
