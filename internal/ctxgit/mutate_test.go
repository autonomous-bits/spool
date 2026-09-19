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
