package commands

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository"
)

func TestCherryPickCLIRequiresCommitAndTargetBranchFlags(t *testing.T) {
	repo := newTestSeedRepository(t)
	var output bytes.Buffer
	command := NewCherryPickCommand(func() (*repository.Repository, error) {
		return repo, nil
	})
	command.SetOut(&output)
	command.SetArgs([]string{})
	err := command.Execute()
	if err == nil {
		t.Fatal("expected error when required flags are missing")
	}

	command.SetArgs([]string{"--commit", "commit-1"})
	err = command.Execute()
	if err == nil {
		t.Fatal("expected error when --target-branch flag is missing")
	}

	command.SetArgs([]string{"--target-branch", "main"})
	err = command.Execute()
	if err == nil {
		t.Fatal("expected error when --commit flag is missing")
	}
}

func TestCherryPickCLIDryRunEmitsJSON(t *testing.T) {
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
			{Action: "add", Entity: "node", ID: "feature-node-1", Title: "Feature 1", Labels: []string{"Product"}},
		},
	})
	if err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	commitRes, err := repo.CommitStagedMutations("feature")
	if err != nil {
		t.Fatalf("CommitStagedMutations: %v", err)
	}

	var output bytes.Buffer
	command := NewCherryPickCommand(func() (*repository.Repository, error) {
		return repo, nil
	})
	command.SetOut(&output)
	command.SetArgs([]string{"--commit", string(commitRes.Commit), "--target-branch", "main", "--dry-run"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute cherry-pick --dry-run: %v", err)
	}

	var result repository.CherryPickResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode cherry-pick JSON: %v", err)
	}
	if !result.DryRun {
		t.Fatal("expected result.DryRun = true")
	}
	if len(result.Changes) != 1 || result.Changes[0].ID != "feature-node-1" || result.Changes[0].Change != "added" {
		t.Fatalf("unexpected changes: %+v", result.Changes)
	}
	if len(result.Conflicts) != 0 || len(result.Violations) != 0 {
		t.Fatalf("expected 0 conflicts/violations, got conflicts=%v violations=%v", result.Conflicts, result.Violations)
	}
}

func TestCherryPickCLIExecutionEmitsJSONAndAdvancesTargetBranch(t *testing.T) {
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
			{Action: "add", Entity: "node", ID: "feature-node-1", Title: "Feature 1", Labels: []string{"Product"}},
		},
	})
	if err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	commitRes, err := repo.CommitStagedMutations("feature")
	if err != nil {
		t.Fatalf("CommitStagedMutations: %v", err)
	}

	var output bytes.Buffer
	command := NewCherryPickCommand(func() (*repository.Repository, error) {
		return repo, nil
	})
	command.SetOut(&output)
	command.SetArgs([]string{
		"--commit", string(commitRes.Commit),
		"--target-branch", "main",
		"--author", "alice@example.com",
		"--message", "Transplant feature 1",
	})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute cherry-pick: %v", err)
	}

	var result repository.CherryPickResult
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("decode cherry-pick JSON: %v", err)
	}
	if result.DryRun {
		t.Fatal("expected result.DryRun = false")
	}
	if len(result.Changes) != 1 || result.Changes[0].ID != "feature-node-1" {
		t.Fatalf("expected 1 change, got %+v", result.Changes)
	}
	if result.Commit == "" {
		t.Fatal("expected non-empty commit hash")
	}
}
