package commands

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/autonomous-bits/spool/internal/repository"
	"github.com/autonomous-bits/spool/internal/resolve"
)

func TestCommitCLIAndMCPCommitEquivalentStaging(t *testing.T) {
	operations := []repository.MutationOperation{{Action: "add", Entity: "node", ID: "node-2", Title: "Second node"}}
	cliRepo := repository.NewSeedRepository()
	if _, err := cliRepo.StageMutationBatch(repository.StageMutationRequest{Branch: "main", Operations: operations}); err != nil {
		t.Fatalf("stage CLI batch: %v", err)
	}

	var output bytes.Buffer
	command := NewCommitCommand(func() (*resolve.ResolveTool, error) { return resolve.NewResolveTool(cliRepo), nil })
	command.SetOut(&output)
	command.SetArgs([]string{"--branch", "main"})
	if err := command.Execute(); err != nil {
		t.Fatalf("execute commit command: %v", err)
	}
	var cliResult repository.CommitStagedMutationResult
	if err := json.Unmarshal(output.Bytes(), &cliResult); err != nil {
		t.Fatalf("decode CLI result: %v", err)
	}

	mcpRepo := repository.NewSeedRepository()
	if _, err := mcpRepo.StageMutationBatch(repository.StageMutationRequest{Branch: "main", Operations: operations}); err != nil {
		t.Fatalf("stage MCP batch: %v", err)
	}
	mcpResult, err := resolve.NewResolveTool(mcpRepo).SPLCommitStagedMutations(context.Background(), "main")
	if err != nil {
		t.Fatalf("SPLCommitStagedMutations: %v", err)
	}
	if cliResult != mcpResult {
		t.Fatalf("CLI result %#v does not match MCP result %#v", cliResult, mcpResult)
	}
}

func TestCommitCLIPersistsCallerMetadataForHistory(t *testing.T) {
	repo := repository.NewSeedRepository()
	if _, err := repo.StageMutationBatch(repository.StageMutationRequest{
		Branch: "main", Operations: []repository.MutationOperation{{Action: "update", Entity: "node", ID: repository.SeedNodeID, Title: "Updated"}},
	}); err != nil {
		t.Fatalf("stage: %v", err)
	}
	command := NewCommitCommand(func() (*resolve.ResolveTool, error) { return resolve.NewResolveTool(repo), nil })
	command.SetArgs([]string{"--branch", "main", "--author", "alice", "--message", "update node"})
	if err := command.Execute(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	history, err := resolve.NewResolveTool(repo).SPLHistory(context.Background(), resolve.HistoryRequest{
		Selector: snapshotSelector("main", ""), EntityID: repository.SeedNodeID,
	})
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if history.Entries[0].Author != "alice" || history.Entries[0].Message != "update node" {
		t.Fatalf("history = %#v", history.Entries[0])
	}
}

func TestCommitCLIRejectsUnstagedBranch(t *testing.T) {
	command := NewCommitCommand(func() (*resolve.ResolveTool, error) {
		return resolve.NewResolveTool(repository.NewSeedRepository()), nil
	})
	command.SetArgs([]string{"--branch", "main"})
	if err := command.Execute(); !errors.Is(err, repository.ErrNoStagedMutations) {
		t.Fatalf("commit command error = %v, want ErrNoStagedMutations", err)
	}
}

func TestCommitCLIRejectsStaleStagedBaseWithoutChangingBranchOrStaging(t *testing.T) {
	repo := repository.NewSeedRepository()
	if _, err := repo.StageMutationBatch(repository.StageMutationRequest{
		Branch:     "main",
		Operations: []repository.MutationOperation{{Action: "add", Entity: "node", ID: "node-2", Title: "Second node"}},
	}); err != nil {
		t.Fatalf("StageMutationBatch: %v", err)
	}
	if _, err := repo.AdvanceBranch("main"); err != nil {
		t.Fatalf("AdvanceBranch: %v", err)
	}
	head, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch before commit: %v", err)
	}
	staging, err := repo.BranchStagingStatus("main")
	if err != nil {
		t.Fatalf("BranchStagingStatus before commit: %v", err)
	}

	command := NewCommitCommand(func() (*resolve.ResolveTool, error) {
		return resolve.NewResolveTool(repo), nil
	})
	command.SetArgs([]string{"--branch", "main"})
	if err := command.Execute(); !errors.Is(err, repository.ErrStaleStagedBase) {
		t.Fatalf("commit command error = %v, want ErrStaleStagedBase", err)
	}

	currentHead, err := repo.PinBranch("main")
	if err != nil {
		t.Fatalf("PinBranch after commit: %v", err)
	}
	currentStaging, err := repo.BranchStagingStatus("main")
	if err != nil {
		t.Fatalf("BranchStagingStatus after commit: %v", err)
	}
	if currentHead != head || currentStaging != staging {
		t.Fatalf("branch/staging = %q/%#v, want unchanged %q/%#v", currentHead, currentStaging, head, staging)
	}
}
