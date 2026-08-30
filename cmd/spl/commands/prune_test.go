package commands

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository"
)

func TestPruneCLIRequiresBranchFlag(t *testing.T) {
	repo := newTestSeedRepository(t)
	var output bytes.Buffer
	command := NewPruneCommand(func() (*repository.Repository, error) {
		return repo, nil
	})
	command.SetOut(&output)
	command.SetArgs([]string{})
	err := command.Execute()
	if err == nil {
		t.Fatal("expected error when --branch flag is missing")
	}
}

func TestPruneCLIDryRunEmitsJSON(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "repo")
	repo, err := repository.InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	_, err = repo.StageMutationBatch(repository.StageMutationRequest{
		Branch: "main",
		Operations: []repository.MutationOperation{
			{Action: "add", Entity: "node", ID: "durable-1", Title: "Durable 1", Labels: []string{"Architecture", "Component"}},
			{Action: "add", Entity: "node", ID: "ephemeral-1", Title: "Ephemeral 1", Labels: []string{"Architecture", "Ephemeral"}},
			{Action: "add", Entity: "edge", ID: "edge-1", Source: "durable-1", Target: "ephemeral-1", Type: "DEPENDS_ON"},
		},
	})
	if err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	if _, err := repo.CommitStagedMutations("main"); err != nil {
		t.Fatalf("CommitStagedMutations: %v", err)
	}

	var output bytes.Buffer
	command := NewPruneCommand(func() (*repository.Repository, error) {
		return repo, nil
	})
	command.SetOut(&output)
	command.SetArgs([]string{"--branch", "main", "--dry-run", "--force"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute prune --dry-run: %v", err)
	}

	var result repository.PruneResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode prune JSON: %v", err)
	}
	if !result.DryRun {
		t.Fatal("expected result.DryRun = true")
	}
	if result.PrunedNodesCount != 1 || len(result.PrunedNodeIDs) != 1 || result.PrunedNodeIDs[0] != "ephemeral-1" {
		t.Fatalf("unexpected pruned nodes: %+v", result)
	}
	if result.PrunedEdgesCount != 1 {
		t.Fatalf("expected 1 pruned edge, got %d", result.PrunedEdgesCount)
	}
}

func TestPruneCLIExecutionEmitsJSONAndAdvancesBranch(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "repo")
	repo, err := repository.InitializeRepository(stateDir)
	if err != nil {
		t.Fatalf("InitializeRepository: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	_, err = repo.CreateBranch("feature", repository.BranchSource{Branch: "main"})
	if err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}

	_, err = repo.StageMutationBatch(repository.StageMutationRequest{
		Branch: "feature",
		Operations: []repository.MutationOperation{
			{Action: "add", Entity: "node", ID: "durable-1", Title: "Durable 1", Labels: []string{"Architecture", "Component"}},
			{Action: "add", Entity: "node", ID: "ephemeral-1", Title: "Ephemeral 1", Labels: []string{"Architecture", "Ephemeral"}},
			{Action: "add", Entity: "edge", ID: "edge-1", Source: "durable-1", Target: "ephemeral-1", Type: "DEPENDS_ON"},
		},
	})
	if err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	if _, err := repo.CommitStagedMutations("feature"); err != nil {
		t.Fatalf("CommitStagedMutations: %v", err)
	}

	var output bytes.Buffer
	command := NewPruneCommand(func() (*repository.Repository, error) {
		return repo, nil
	})
	command.SetOut(&output)
	command.SetArgs([]string{"--branch", "feature", "--author", "alice", "--message", "Clean scaffold"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute prune: %v", err)
	}

	var result repository.PruneResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode prune JSON: %v", err)
	}
	if result.DryRun {
		t.Fatal("expected result.DryRun = false")
	}
	if result.PrunedNodesCount != 1 || result.PrunedEdgesCount != 1 {
		t.Fatalf("expected 1 node and 1 edge pruned, got %+v", result)
	}
	if result.Commit == "" {
		t.Fatal("expected non-empty commit hash")
	}
}
